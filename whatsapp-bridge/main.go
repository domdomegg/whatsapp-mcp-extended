// whatsapp-mcp: one WhatsApp account as an MCP server over stdio.
//
// mcp-auth-wrapper spawns one of these per user profile with MCP_USER_ID and
// MCP_PROFILE_ID set; state lives under DATA_ROOT/store/<user>/<profile>.
// `whatsapp-mcp healthcheck` is the container's liveness probe.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"whatsapp-bridge/internal/config"
	"whatsapp-bridge/internal/database"
	"whatsapp-bridge/internal/health"
	mcpserver "whatsapp-bridge/internal/mcp"
	"whatsapp-bridge/internal/whatsapp"
)

const (
	presencePingInterval = 20 * time.Minute // below ~25m risks bot fingerprinting; keeps the session warm
	watchdogLimit        = 3 * time.Minute  // disconnected this long: exit, the wrapper respawns on demand
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

// healthcheck is the exec liveness probe: non-zero when a linked account in
// this container has been disconnected for too long or a process has hung.
func healthcheck() int {
	root := os.Getenv("DATA_ROOT")
	if root == "" {
		root = "/app/data"
	}
	problems := health.Check(root, time.Now(), health.Alive)
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, "[healthcheck]", p)
	}
	if len(problems) > 0 {
		return 1
	}
	return 0
}

func run() error {
	logger := newLogger("whatsapp", os.Getenv("DEBUG") == "true")
	storeDir := config.StoreDir()
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return fmt.Errorf("create store directory %s: %w", storeDir, err)
	}
	logger.Infof("store %s", storeDir)

	messageStore, err := database.NewMessageStore()
	if err != nil {
		return fmt.Errorf("open message store: %w", err)
	}
	defer messageStore.Close()

	client, err := whatsapp.NewClient(logger)
	if err != nil {
		return fmt.Errorf("create WhatsApp client: %w", err)
	}

	// whatsmeow has created whatsapp.db by now; open it read-only for contacts.
	contacts, err := database.OpenContactStore()
	if err != nil {
		return fmt.Errorf("open contact store: %w", err)
	}
	defer contacts.Close()

	client.AddEventHandler(eventHandler(client, messageStore, logger))

	// Connect in the background: the MCP server must answer initialize at once,
	// and a not-yet-linked account needs the QR tools to be reachable.
	go func() {
		if err := client.Connect(); err != nil {
			logger.Errorf("connect: %v", err)
		}
	}()

	go watchdog(client, logger)
	go presencePing(client, logger)
	go writeHealth(client, storeDir, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := mcpserver.New(client, messageStore, contacts, config.MediaDir())
	err = server.Run(ctx)

	logger.Infof("shutting down")
	client.Disconnect()
	return err
}

// watchdog exits the process when WhatsApp has been gone for watchdogLimit.
// With one process per account that is the whole recovery story: the wrapper's
// stdio session ends and the next tool call starts a fresh process.
func watchdog(client *whatsapp.Client, logger waLog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		_, _, discAt, _ := client.ConnectionState()
		if !discAt.IsZero() && time.Since(discAt) > watchdogLimit {
			logger.Errorf("WATCHDOG: disconnected for %v, exiting", time.Since(discAt).Round(time.Second))
			os.Exit(1)
		}
	}
}

func presencePing(client *whatsapp.Client, logger waLog.Logger) {
	ticker := time.NewTicker(presencePingInterval)
	defer ticker.Stop()
	for range ticker.C {
		if client.IsConnected() {
			if err := client.SetPresence("available"); err != nil {
				logger.Debugf("presence ping failed: %v", err)
			}
		}
	}
}

func writeHealth(client *whatsapp.Client, storeDir string, logger waLog.Logger) {
	write := func() {
		_, _, discAt, _ := client.ConnectionState()
		var disconnectedFor int64
		if !discAt.IsZero() {
			disconnectedFor = int64(time.Since(discAt).Seconds())
		}
		err := health.Write(storeDir, health.Status{
			PID: os.Getpid(), Connected: client.IsConnected(), NeedsPairing: client.Store.ID == nil,
			DisconnectedForSeconds: disconnectedFor,
		})
		if err != nil {
			logger.Warnf("write health file: %v", err)
		}
	}
	write()
	ticker := time.NewTicker(health.WriteInterval)
	defer ticker.Stop()
	for range ticker.C {
		write()
	}
}

// activeCall tracks an ongoing call for duration/status calculation.
type activeCall struct {
	ChatJID   string
	Sender    string
	Name      string
	Timestamp time.Time
	IsFromMe  bool
}

// formatDuration formats a duration as "M:SS".
func formatDuration(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 0 {
		secs = 0
	}
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

// resolveCallJID converts LID (hidden user) JIDs to regular phone JIDs.
func resolveCallJID(client *whatsapp.Client, logger waLog.Logger, jid types.JID) types.JID {
	if jid.Server == types.HiddenUserServer {
		resolved, err := client.Store.GetAltJID(context.Background(), jid)
		if err == nil && !resolved.IsEmpty() {
			return resolved
		}
		logger.Warnf("[CALL] Could not resolve LID %s: %v", jid, err)
	}
	return jid
}

// isCallFromMe checks if a call originated from this device.
func isCallFromMe(client *whatsapp.Client, from, resolvedFrom, callCreator types.JID) bool {
	if client.Store.ID == nil {
		return false
	}
	own := client.Store.ID.ToNonAD().User
	return from.User == own || resolvedFrom.User == own || (!callCreator.IsEmpty() && callCreator.User == own)
}

// eventHandler stores messages, history and calls, and keeps the connection
// state the watchdog and health file read.
func eventHandler(client *whatsapp.Client, messageStore *database.MessageStore, logger waLog.Logger) func(any) {
	activeCalls := map[string]*activeCall{}
	var mu sync.Mutex

	startCall := func(callID string, from, groupJID types.JID, creator types.JID, ts time.Time, media string) {
		resolved := resolveCallJID(client, logger, from)
		callFrom := resolved.User
		fromMe := isCallFromMe(client, from, resolved, creator)
		chatJID, chatResolved := resolved.String(), resolved
		if !groupJID.IsEmpty() {
			chatJID, chatResolved = groupJID.String(), groupJID
		}
		name := client.GetChatName(messageStore, chatResolved, chatJID, nil, callFrom)
		kind := "call"
		if media != "" {
			kind = media + " call"
		}
		content := fmt.Sprintf("📞 Incoming %s from %s", kind, name)
		if fromMe {
			content = fmt.Sprintf("📞 Outgoing %s to %s", kind, name)
		}
		mu.Lock()
		activeCalls[callID] = &activeCall{ChatJID: chatJID, Sender: callFrom, Name: name, Timestamp: ts, IsFromMe: fromMe}
		mu.Unlock()
		if err := messageStore.StoreChat(chatJID, name, ts); err != nil {
			logger.Warnf("store chat for call: %v", err)
		}
		if err := messageStore.StoreMessage("call-"+callID, chatJID, callFrom, name, content, ts, fromMe, "call", "", "", "", nil, nil, nil, 0); err != nil {
			logger.Warnf("store call message: %v", err)
		}
	}
	endCall := func(callID string, ts time.Time, missed bool) {
		mu.Lock()
		call, ok := activeCalls[callID]
		delete(activeCalls, callID)
		mu.Unlock()
		if !ok {
			return
		}
		content := fmt.Sprintf("📞 Missed call from %s", call.Name)
		if !missed {
			content = fmt.Sprintf("📞 Call with %s (%s)", call.Name, formatDuration(ts.Sub(call.Timestamp)))
		}
		if err := messageStore.StoreMessage("call-"+callID, call.ChatJID, call.Sender, call.Name, content, call.Timestamp, call.IsFromMe, "call", "", "", "", nil, nil, nil, 0); err != nil {
			logger.Warnf("update call message: %v", err)
		}
	}

	go func() { // forget calls that never got a terminate event
		for range time.Tick(time.Minute) {
			cutoff := time.Now().Add(-5 * time.Minute)
			mu.Lock()
			for id, call := range activeCalls {
				if call.Timestamp.Before(cutoff) {
					delete(activeCalls, id)
				}
			}
			mu.Unlock()
		}
	}()

	return func(evt any) {
		switch v := evt.(type) {
		case *events.Message:
			client.HandleMessage(messageStore, v)

		case *events.HistorySync:
			logger.Infof("[SYNC] history sync (type %v, %d conversations)", v.Data.SyncType, len(v.Data.Conversations))
			client.HandleHistorySync(messageStore, v)

		case *events.Connected:
			_, _, discAt, _ := client.ConnectionState()
			client.MarkConnected()
			if err := client.SetPresence("available"); err != nil {
				logger.Warnf("set presence: %v", err)
			}
			logger.Infof("✓ Connected to WhatsApp")
			if !discAt.IsZero() && time.Since(discAt) > 30*time.Second {
				go backfillAfterGap(client, messageStore, logger, time.Since(discAt))
			}

		case *events.LoggedOut:
			logger.Warnf("✗ Device logged out — credentials wiped, call get_setup_qr to re-link")
			client.MarkDisconnected()

		case *events.PairSuccess:
			logger.Infof("✓ Phone pairing successful")
			client.HandlePairingSuccess()

		case *events.PairError:
			logger.Errorf("✗ Phone pairing failed: %v", v.Error)
			client.HandlePairingError(v.Error)

		case *events.KeepAliveTimeout:
			logger.Warnf("⚠ KeepAlive timeout (errors: %d)", v.ErrorCount)
			if v.ErrorCount >= 3 {
				logger.Errorf("KeepAlive: %d consecutive failures, forcing disconnect+reconnect", v.ErrorCount)
				client.Disconnect()
				go func() {
					time.Sleep(2 * time.Second)
					if err := client.Client.Connect(); err != nil {
						logger.Errorf("reconnect after KeepAlive failure: %v", err)
					}
				}()
			}

		case *events.KeepAliveRestored:
			logger.Infof("✓ KeepAlive restored")

		case *events.StreamReplaced:
			// Another process took this session; two of them would split-brain.
			logger.Errorf("✗ Stream replaced — another process took this session, exiting")
			client.MarkDisconnected()
			os.Exit(1)

		case *events.StreamError:
			logger.Errorf("✗ Stream error: %v", v.Code)

		case *events.Disconnected:
			client.MarkDisconnected()
			logger.Warnf("⚠ Disconnected from WhatsApp — reconnecting")

		case *events.CallOffer:
			startCall(v.CallID, v.From, v.GroupJID, v.CallCreator, v.Timestamp, "")
		case *events.CallOfferNotice:
			startCall(v.CallID, v.From, v.GroupJID, v.CallCreator, v.Timestamp, v.Media)
		case *events.CallAccept:
			mu.Lock()
			if call, ok := activeCalls[v.CallID]; ok {
				call.Timestamp = v.Timestamp
			}
			mu.Unlock()
		case *events.CallTerminate:
			endCall(v.CallID, v.Timestamp, v.Reason == "timeout" || v.Reason == "busy")
		case *events.CallReject:
			endCall(v.CallID, v.Timestamp, true)
		}
	}
}

// backfillAfterGap asks the phone for recent history of chats active in the
// last day, to recover messages missed while disconnected.
func backfillAfterGap(client *whatsapp.Client, messageStore *database.MessageStore, logger waLog.Logger, gap time.Duration) {
	logger.Warnf("[RECONNECT] offline for %v — requesting history backfill", gap.Round(time.Second))
	time.Sleep(5 * time.Second) // let the session settle first
	chats, err := messageStore.GetChats()
	if err != nil {
		logger.Warnf("[RECONNECT] list chats: %v", err)
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	requested := 0
	for chatJID, lastMsgTime := range chats {
		if lastMsgTime.Before(cutoff) {
			continue
		}
		msgs, err := messageStore.GetMessages(chatJID, 1)
		if err != nil || len(msgs) == 0 {
			continue
		}
		newest := msgs[0]
		if err := client.RequestChatHistory(chatJID, newest.ID, newest.IsFromMe, newest.Sender, newest.Time.UnixMilli(), 50); err != nil {
			logger.Warnf("[RECONNECT] history request for %s: %v", chatJID, err)
		} else {
			requested++
		}
		time.Sleep(500 * time.Millisecond)
	}
	logger.Infof("[RECONNECT] backfill requested for %d active chats", requested)
}
