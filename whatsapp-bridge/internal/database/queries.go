package database

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Output shapes for the MCP tools. Field names and the omit-when-empty rules
// mirror what the Python server produced, so clients and prompts written
// against it keep working. Nullable fields that Python always emitted (chat
// name, contact name) are pointers so they serialise as null, not "".

// MessageOut is one message as returned by list_messages & co.
type MessageOut struct {
	ID                string            `json:"id"`
	ChatJID           string            `json:"chat_jid"`
	Timestamp         string            `json:"timestamp"`
	Sender            string            `json:"sender"`
	Content           string            `json:"content"`
	IsFromMe          bool              `json:"is_from_me"`
	IsGroup           bool              `json:"is_group"`
	ChatName          string            `json:"chat_name,omitempty"`
	SenderName        string            `json:"sender_name,omitempty"`
	MediaType         string            `json:"media_type,omitempty"`
	Filename          *string           `json:"filename,omitempty"`
	FileLength        *int64            `json:"file_length,omitempty"`
	SenderContactInfo map[string]string `json:"sender_contact_info,omitempty"`
	CharacterCount    *int              `json:"character_count,omitempty"`
	WordCount         *int              `json:"word_count,omitempty"`
	URLList           []string          `json:"url_list,omitempty"`
	ReplyToMessageID  string            `json:"reply_to_message_id,omitempty"`
	QuotedMessageID   string            `json:"quoted_message_id,omitempty"`
	QuotedSenderName  string            `json:"quoted_sender_name,omitempty"`
	EditCount         int               `json:"edit_count,omitempty"`
	IsEdited          bool              `json:"is_edited,omitempty"`
	IsForwarded       bool              `json:"is_forwarded,omitempty"`
	ForwardedFrom     string            `json:"forwarded_from,omitempty"`
	IsSystemMessage   bool              `json:"is_system_message,omitempty"`
	SystemMessageType string            `json:"system_message_type,omitempty"`
}

// MessageContextOut is a message with its neighbours.
type MessageContextOut struct {
	Message MessageOut   `json:"message"`
	Before  []MessageOut `json:"before"`
	After   []MessageOut `json:"after"`
}

// ChatOut is one chat as returned by list_chats & co.
type ChatOut struct {
	JID                      string            `json:"jid"`
	Name                     *string           `json:"name"`
	IsGroup                  bool              `json:"is_group"`
	LastMessageTime          string            `json:"last_message_time,omitempty"`
	LastMessage              string            `json:"last_message,omitempty"`
	LastMessageID            string            `json:"last_message_id,omitempty"`
	LastSender               string            `json:"last_sender,omitempty"`
	LastSenderName           string            `json:"last_sender_name,omitempty"`
	LastIsFromMe             *bool             `json:"last_is_from_me,omitempty"`
	TotalMessageCount        *int              `json:"total_message_count,omitempty"`
	MessageCountToday        *int              `json:"message_count_today,omitempty"`
	MessageCountLast7Days    *int              `json:"message_count_last_7_days,omitempty"`
	MessageVelocityLast7Days map[string]any    `json:"message_velocity_last_7_days,omitempty"`
	MostActiveMemberName     string            `json:"most_active_member_name,omitempty"`
	MostActiveMemberMsgCount *int              `json:"most_active_member_message_count,omitempty"`
	ChatType                 string            `json:"chat_type,omitempty"`
	SilentDurationSeconds    *int64            `json:"silent_duration_seconds,omitempty"`
	IsRecentlyActive         bool              `json:"is_recently_active,omitempty"`
	MediaCountByType         map[string]int    `json:"media_count_by_type,omitempty"`
	RecentMedia              []string          `json:"recent_media,omitempty"`
	HasMedia                 bool              `json:"has_media,omitempty"`
	LastSenderContactInfo    map[string]string `json:"last_sender_contact_info,omitempty"`
}

// ContactOut is one contact as returned by search_contacts & co.
type ContactOut struct {
	JID          string  `json:"jid"`
	PhoneNumber  string  `json:"phone_number"`
	Name         *string `json:"name"`
	FirstName    string  `json:"first_name,omitempty"`
	FullName     string  `json:"full_name,omitempty"`
	PushName     string  `json:"push_name,omitempty"`
	BusinessName string  `json:"business_name,omitempty"`
	Nickname     string  `json:"nickname,omitempty"`
}

// ErrNotFound is returned when a looked-up row does not exist.
var ErrNotFound = errors.New("not found")

var urlPattern = regexp.MustCompile(`https?://[^\s]+`)

func wordCount(s string) int   { return len(strings.Fields(s)) }
func charCount(s string) int   { return len([]rune(s)) }
func isGroupJID(j string) bool { return strings.HasSuffix(j, "@g.us") }

// formatTime renders like Python's datetime.isoformat(): fractional seconds
// only when non-zero, "+00:00" rather than "Z".
func formatTime(t time.Time) string {
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04:05-07:00")
	}
	return t.Format("2006-01-02T15:04:05.999999-07:00")
}

// ParseTime accepts the ISO-8601 forms clients send for after/before.
func ParseTime(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date format: %q", s)
}

func ptr[T any](v T) *T { return &v }

// contactInfo is the small sender block Python attached to messages: jid,
// phone (non-groups) and, when a nickname is set, name + nickname.
func (store *MessageStore) contactInfo(jid string) map[string]string {
	if jid == "" {
		return nil
	}
	info := map[string]string{"jid": jid}
	if !isGroupJID(jid) {
		if phone := PhoneFromJID(jid); phone != "" {
			info["phone_number"] = phone
		}
	}
	if nick, _ := store.GetNickname(jid); nick != "" {
		info["name"] = nick
		info["nickname"] = nick
	}
	return info
}

// senderName falls back to the sender JID's user part when nothing is stored.
func senderName(stored, sender string) string {
	if stored != "" {
		return stored
	}
	return PhoneFromJID(sender)
}

const messageSelect = `SELECT messages.timestamp, messages.sender, COALESCE(chats.name,''), COALESCE(messages.content,''),
	messages.is_from_me, chats.jid, messages.id, COALESCE(messages.media_type,''), messages.filename, messages.file_length,
	COALESCE(messages.sender_name,''), COALESCE(messages.quoted_message_id,''), COALESCE(messages.quoted_sender_name,''),
	COALESCE(messages.reply_to_message_id,''), COALESCE(messages.edit_count,0), COALESCE(messages.is_edited,0),
	COALESCE(messages.is_forwarded,0), COALESCE(messages.forwarded_from,''), COALESCE(messages.is_system_message,0),
	COALESCE(messages.system_message_type,'')
	FROM messages JOIN chats ON messages.chat_jid = chats.jid`

func (store *MessageStore) scanMessage(rows *sql.Rows) (MessageOut, error) {
	var (
		m        MessageOut
		ts       time.Time
		filename sql.NullString
		length   sql.NullInt64
	)
	if err := rows.Scan(&ts, &m.Sender, &m.ChatName, &m.Content, &m.IsFromMe, &m.ChatJID, &m.ID, &m.MediaType, &filename, &length,
		&m.SenderName, &m.QuotedMessageID, &m.QuotedSenderName, &m.ReplyToMessageID, &m.EditCount, &m.IsEdited,
		&m.IsForwarded, &m.ForwardedFrom, &m.IsSystemMessage, &m.SystemMessageType); err != nil {
		return m, err
	}
	m.Timestamp = formatTime(ts)
	m.SenderName = senderName(m.SenderName, m.Sender)
	m.IsGroup = isGroupJID(m.ChatJID)
	m.CharacterCount = ptr(charCount(m.Content))
	m.WordCount = ptr(wordCount(m.Content))
	m.URLList = urlPattern.FindAllString(m.Content, -1)
	m.SenderContactInfo = store.contactInfo(m.Sender)
	if m.MediaType != "" {
		if filename.Valid {
			m.Filename = ptr(filename.String)
		} else {
			m.Filename = ptr("")
		}
		if length.Valid {
			m.FileLength = ptr(length.Int64)
		} else {
			m.FileLength = ptr(int64(0))
		}
	}
	return m, nil
}

func (store *MessageStore) queryMessages(query string, args ...any) ([]MessageOut, error) {
	rows, err := store.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MessageOut{}
	for rows.Next() {
		m, err := store.scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListMessagesOptions are the list_messages arguments.
type ListMessagesOptions struct {
	After, Before  *time.Time
	ChatJID, Query string
	Limit, Page    int
	IncludeContext bool
	ContextBefore  int
	ContextAfter   int
}

// ListMessages returns messages newest first, optionally with neighbours
// inlined (deduplicated, in the order Python produced them).
func (store *MessageStore) ListMessages(o ListMessagesOptions) ([]MessageOut, error) {
	var where []string
	var args []any
	if o.After != nil {
		where = append(where, "messages.timestamp > ?")
		args = append(args, *o.After)
	}
	if o.Before != nil {
		where = append(where, "messages.timestamp < ?")
		args = append(args, *o.Before)
	}
	if o.ChatJID != "" {
		where = append(where, "messages.chat_jid = ?")
		args = append(args, o.ChatJID)
	}
	if o.Query != "" {
		where = append(where, "LOWER(messages.content) LIKE LOWER(?)")
		args = append(args, "%"+o.Query+"%")
	}
	q := messageSelect
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY messages.timestamp DESC LIMIT ? OFFSET ?"
	args = append(args, o.Limit, o.Page*o.Limit)
	result, err := store.queryMessages(q, args...)
	if err != nil {
		return nil, err
	}
	if !o.IncludeContext || len(result) == 0 {
		return result, nil
	}
	seen := map[string]bool{}
	var withContext []MessageOut
	add := func(m MessageOut) {
		if !seen[m.ID] {
			seen[m.ID] = true
			withContext = append(withContext, m)
		}
	}
	for _, m := range result {
		ctx, err := store.GetMessageContext(m.ID, o.ContextBefore, o.ContextAfter)
		if err != nil {
			return nil, err
		}
		for _, b := range ctx.Before {
			add(b)
		}
		add(ctx.Message)
		for _, a := range ctx.After {
			add(a)
		}
	}
	return withContext, nil
}

// GetMessageContext returns a message with up to `before` earlier and `after`
// later messages from the same chat, each in chronological order.
func (store *MessageStore) GetMessageContext(messageID string, before, after int) (MessageContextOut, error) {
	target, err := store.queryMessages(messageSelect+" WHERE messages.id = ? LIMIT 1", messageID)
	if err != nil {
		return MessageContextOut{}, err
	}
	if len(target) == 0 {
		return MessageContextOut{}, fmt.Errorf("message with ID %s %w", messageID, ErrNotFound)
	}
	var ts time.Time
	if err := store.db.QueryRow("SELECT timestamp FROM messages WHERE id = ? LIMIT 1", messageID).Scan(&ts); err != nil {
		return MessageContextOut{}, err
	}
	chat := target[0].ChatJID
	earlier, err := store.queryMessages(messageSelect+" WHERE messages.chat_jid = ? AND messages.timestamp < ? ORDER BY messages.timestamp DESC LIMIT ?", chat, ts, before)
	if err != nil {
		return MessageContextOut{}, err
	}
	for i, j := 0, len(earlier)-1; i < j; i, j = i+1, j-1 {
		earlier[i], earlier[j] = earlier[j], earlier[i]
	}
	later, err := store.queryMessages(messageSelect+" WHERE messages.chat_jid = ? AND messages.timestamp > ? ORDER BY messages.timestamp ASC LIMIT ?", chat, ts, after)
	if err != nil {
		return MessageContextOut{}, err
	}
	return MessageContextOut{Message: target[0], Before: earlier, After: later}, nil
}

// ListChatsOptions are the list_chats arguments.
type ListChatsOptions struct {
	Query              string
	Limit, Page        int
	IncludeLastMessage bool
	SortByName         bool
}

func (store *MessageStore) chatStatistics(chatJID string) (total, today, week int, err error) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if err = store.db.QueryRow("SELECT COUNT(*) FROM messages WHERE chat_jid = ?", chatJID).Scan(&total); err != nil {
		return
	}
	if err = store.db.QueryRow("SELECT COUNT(*) FROM messages WHERE chat_jid = ? AND timestamp >= ?", chatJID, todayStart).Scan(&today); err != nil {
		return
	}
	err = store.db.QueryRow("SELECT COUNT(*) FROM messages WHERE chat_jid = ? AND timestamp >= ?", chatJID, now.AddDate(0, 0, -7)).Scan(&week)
	return
}

func (store *MessageStore) mostActiveMember(chatJID string) (string, int) {
	var name string
	var count int
	err := store.db.QueryRow(`SELECT sender_name, COUNT(*) AS count FROM messages
		WHERE chat_jid = ? AND sender_name != '' AND sender_name IS NOT NULL
		GROUP BY sender_name ORDER BY count DESC LIMIT 1`, chatJID).Scan(&name, &count)
	if err != nil {
		return "", 0
	}
	return name, count
}

func (store *MessageStore) mediaStats(chatJID string) (map[string]int, []string) {
	byType := map[string]int{}
	rows, err := store.db.Query(`SELECT media_type, COUNT(*) FROM messages
		WHERE chat_jid = ? AND media_type != '' AND media_type IS NOT NULL GROUP BY media_type`, chatJID)
	if err == nil {
		for rows.Next() {
			var t string
			var n int
			if rows.Scan(&t, &n) == nil {
				byType[t] = n
			}
		}
		rows.Close()
	}
	var recent []string
	rows, err = store.db.Query(`SELECT COALESCE(filename,'') FROM messages
		WHERE chat_jid = ? AND media_type != '' AND media_type IS NOT NULL ORDER BY timestamp DESC LIMIT 5`, chatJID)
	if err == nil {
		for rows.Next() {
			var f string
			if rows.Scan(&f) == nil && f != "" {
				recent = append(recent, f)
			}
		}
		rows.Close()
	}
	return byType, recent
}

// ListChats returns chats with their latest message and activity statistics.
func (store *MessageStore) ListChats(o ListChatsOptions) ([]ChatOut, error) {
	q := `SELECT c.jid, c.name, m.timestamp, COALESCE(m.content,''), COALESCE(m.id,''), COALESCE(m.sender,''), m.is_from_me, COALESCE(m.sender_name,'')
		FROM chats c
		LEFT JOIN (
			SELECT chat_jid, timestamp, content, id, sender, is_from_me, sender_name,
				ROW_NUMBER() OVER (PARTITION BY chat_jid ORDER BY timestamp DESC, id DESC) AS rn
			FROM messages
		) m ON c.jid = m.chat_jid AND m.rn = 1`
	var args []any
	if o.Query != "" {
		q += " WHERE (LOWER(c.name) LIKE LOWER(?) OR LOWER(c.jid) LIKE LOWER(?))"
		args = append(args, "%"+o.Query+"%", "%"+o.Query+"%")
	}
	if o.SortByName {
		q += " ORDER BY c.name ASC"
	} else {
		q += " ORDER BY m.timestamp DESC NULLS LAST"
	}
	q += " LIMIT ? OFFSET ?"
	args = append(args, o.Limit, o.Page*o.Limit)
	rows, err := store.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatOut{}
	for rows.Next() {
		var (
			c          ChatOut
			name       sql.NullString
			ts         sql.NullTime
			lastIsMe   sql.NullBool
			lastSender string
			content    string
			lastID     string
			senderNm   string
		)
		if err := rows.Scan(&c.JID, &name, &ts, &content, &lastID, &lastSender, &lastIsMe, &senderNm); err != nil {
			return nil, err
		}
		if name.Valid {
			c.Name = ptr(name.String)
		}
		c.IsGroup = isGroupJID(c.JID)
		if o.IncludeLastMessage {
			c.LastMessage = content
			c.LastMessageID = lastID
			c.LastSender = lastSender
			if lastIsMe.Valid {
				c.LastIsFromMe = ptr(lastIsMe.Bool)
			}
			if lastSender != "" {
				c.LastSenderName = senderName(senderNm, lastSender)
			}
		}
		total, today, week, err := store.chatStatistics(c.JID)
		if err != nil {
			return nil, err
		}
		c.TotalMessageCount, c.MessageCountToday, c.MessageCountLast7Days = ptr(total), ptr(today), ptr(week)
		if c.IsGroup {
			c.ChatType = "group"
		} else {
			c.ChatType = "individual"
		}
		if n, cnt := store.mostActiveMember(c.JID); n != "" {
			c.MostActiveMemberName = n
			if cnt > 0 {
				c.MostActiveMemberMsgCount = ptr(cnt)
			}
		}
		c.MediaCountByType, c.RecentMedia = store.mediaStats(c.JID)
		c.HasMedia = len(c.MediaCountByType) > 0
		if lastSender != "" {
			c.LastSenderContactInfo = store.contactInfo(lastSender)
		}
		if week > 0 {
			avg := float64(week) / 7
			velocity := map[string]any{"messages_per_day": float64(int(avg*100+0.5)) / 100}
			switch {
			case float64(today) > avg*1.2:
				velocity["trend_direction"] = "increasing"
			case float64(today) < avg*0.8:
				velocity["trend_direction"] = "decreasing"
			default:
				velocity["trend_direction"] = "stable"
			}
			c.MessageVelocityLast7Days = velocity
		}
		if ts.Valid {
			c.LastMessageTime = formatTime(ts.Time)
			silent := time.Since(ts.Time)
			c.SilentDurationSeconds = ptr(int64(silent.Seconds()))
			c.IsRecentlyActive = silent < 24*time.Hour
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

const chatWithLastSelect = `SELECT c.jid, c.name, c.last_message_time, COALESCE(lm.content,''), COALESCE(lm.id,''), COALESCE(lm.sender,''), lm.is_from_me
	FROM chats c
	LEFT JOIN (
		SELECT chat_jid, content, id, sender, is_from_me,
			ROW_NUMBER() OVER (PARTITION BY chat_jid ORDER BY timestamp DESC, id DESC) AS rn
		FROM messages
	) lm ON c.jid = lm.chat_jid AND lm.rn = 1`

func (store *MessageStore) scanChatBasic(row interface{ Scan(...any) error }, includeLast bool) (*ChatOut, error) {
	var (
		c        ChatOut
		name     sql.NullString
		ts       sql.NullTime
		content  string
		lastID   string
		sender   string
		lastIsMe sql.NullBool
	)
	if err := row.Scan(&c.JID, &name, &ts, &content, &lastID, &sender, &lastIsMe); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if name.Valid {
		c.Name = ptr(name.String)
	}
	c.IsGroup = isGroupJID(c.JID)
	if ts.Valid {
		c.LastMessageTime = formatTime(ts.Time)
	}
	if includeLast {
		c.LastMessage = content
		c.LastMessageID = lastID
		c.LastSender = sender
		if lastIsMe.Valid {
			c.LastIsFromMe = ptr(lastIsMe.Bool)
		}
	}
	return &c, nil
}

// GetChat returns one chat by JID with its last message, or nil.
func (store *MessageStore) GetChat(chatJID string, includeLastMessage bool) (*ChatOut, error) {
	row := store.db.QueryRow(chatWithLastSelect+" WHERE c.jid = ?", chatJID)
	c, err := store.scanChatBasic(row, includeLastMessage)
	if c != nil {
		c.LastMessageID = "" // Python's get_chat did not select the id
	}
	return c, err
}

// GetDirectChatByContact finds the non-group chat whose JID contains the phone number.
func (store *MessageStore) GetDirectChatByContact(phone string) (*ChatOut, error) {
	row := store.db.QueryRow(chatWithLastSelect+" WHERE c.jid LIKE ? AND c.jid NOT LIKE '%@g.us' LIMIT 1", "%"+phone+"%")
	c, err := store.scanChatBasic(row, true)
	if c != nil {
		c.LastMessageID = ""
	}
	return c, err
}

// GetContactChats lists chats the JID has sent to or is, newest first.
func (store *MessageStore) GetContactChats(jid string, limit, page int) ([]ChatOut, error) {
	rows, err := store.db.Query(`SELECT DISTINCT c.jid, c.name, c.last_message_time, COALESCE(m.content,''), COALESCE(m.id,''), COALESCE(m.sender,''), m.is_from_me
		FROM chats c JOIN messages m ON c.jid = m.chat_jid
		WHERE m.sender = ? OR c.jid = ?
		ORDER BY c.last_message_time DESC LIMIT ? OFFSET ?`, jid, jid, limit, page*limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatOut{}
	for rows.Next() {
		c, err := store.scanChatBasic(rows, true)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetLastInteraction is the newest message sent by, or in the chat of, the JID.
func (store *MessageStore) GetLastInteraction(jid string) (*MessageOut, error) {
	msgs, err := store.queryMessages(messageSelect+" WHERE messages.sender = ? OR chats.jid = ? ORDER BY messages.timestamp DESC LIMIT 1", jid, jid)
	if err != nil || len(msgs) == 0 {
		return nil, err
	}
	m := msgs[0]
	// Python's get_last_interaction built a bare Message: no metadata extras.
	m.CharacterCount, m.WordCount, m.URLList, m.SenderContactInfo = nil, nil, nil, nil
	m.SenderName = ""
	return &m, nil
}

// GetNickname returns the custom nickname for a JID, or "".
func (store *MessageStore) GetNickname(jid string) (string, error) {
	var n string
	err := store.db.QueryRow("SELECT nickname FROM contact_nicknames WHERE jid = ?", jid).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return n, err
}

// SetNickname upserts a nickname.
func (store *MessageStore) SetNickname(jid, nickname string) error {
	_, err := store.db.Exec("INSERT OR REPLACE INTO contact_nicknames (jid, nickname, updated_at) VALUES (?, ?, datetime('now'))", jid, nickname)
	return err
}

// RemoveNickname deletes a nickname; found reports whether one existed.
func (store *MessageStore) RemoveNickname(jid string) (found bool, err error) {
	res, err := store.db.Exec("DELETE FROM contact_nicknames WHERE jid = ?", jid)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// NicknameOut is one contact_nicknames row.
type NicknameOut struct {
	JID       string `json:"jid"`
	Nickname  string `json:"nickname"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ListNicknames returns every nickname, ordered by nickname.
func (store *MessageStore) ListNicknames() ([]NicknameOut, error) {
	rows, err := store.db.Query("SELECT jid, nickname, COALESCE(created_at,''), COALESCE(updated_at,'') FROM contact_nicknames ORDER BY nickname")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NicknameOut{}
	for rows.Next() {
		var n NicknameOut
		if err := rows.Scan(&n.JID, &n.Nickname, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ContactFromRow shapes a whatsmeow contact plus its nickname the way
// search_contacts did: name = full name, else push name, else first name.
func ContactFromRow(r ContactRow, nickname string) ContactOut {
	c := ContactOut{
		JID: r.JID, PhoneNumber: PhoneFromJID(r.JID),
		FirstName: r.FirstName, FullName: r.FullName, PushName: r.PushName, BusinessName: r.BusinessName, Nickname: nickname,
	}
	for _, n := range []string{r.FullName, r.PushName, r.FirstName} {
		if n != "" {
			c.Name = ptr(n)
			break
		}
	}
	return c
}

// SyncStatus is the sync-status resource: counts and the newest message time.
func (store *MessageStore) SyncStatus() (messages, chats int, lastSync string) {
	messages, _ = store.GetMessageCount()
	chats, _ = store.GetChatCount()
	var ts sql.NullTime
	if err := store.db.QueryRow("SELECT MAX(timestamp) FROM messages").Scan(&ts); err == nil && ts.Valid {
		lastSync = ts.Time.Format(time.RFC3339)
	}
	return
}
