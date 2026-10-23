package mcp

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"

	localTypes "whatsapp-bridge/internal/types"
)

type sendMessageIn struct {
	Recipient     string   `json:"recipient" jsonschema:"Phone number with country code (digits only) or a JID such as 123456789@s.whatsapp.net or 123456789@g.us for a group"`
	Message       string   `json:"message" jsonschema:"The message text to send"`
	MentionedJIDs []string `json:"mentioned_jids,omitempty" jsonschema:"Optional list of JIDs to mention in the message"`
}

type sendReactionIn struct {
	ChatJID   string `json:"chat_jid" jsonschema:"The JID of the chat containing the message"`
	MessageID string `json:"message_id" jsonschema:"The ID of the message to react to"`
	Emoji     string `json:"emoji" jsonschema:"The emoji to react with (empty string to remove the reaction)"`
}

type editMessageIn struct {
	ChatJID    string `json:"chat_jid" jsonschema:"The JID of the chat containing the message"`
	MessageID  string `json:"message_id" jsonschema:"The ID of the message to edit (must be your own)"`
	NewContent string `json:"new_content" jsonschema:"The new message content"`
}

type deleteMessageIn struct {
	ChatJID   string `json:"chat_jid" jsonschema:"The JID of the chat containing the message"`
	MessageID string `json:"message_id" jsonschema:"The ID of the message to delete"`
	SenderJID string `json:"sender_jid,omitempty" jsonschema:"Optional sender JID for admins revoking others' messages in groups"`
}

type groupJIDIn struct {
	GroupJID string `json:"group_jid" jsonschema:"The JID of the group (e.g. 123456789@g.us)"`
}

type markReadIn struct {
	ChatJID    string   `json:"chat_jid" jsonschema:"The JID of the chat containing the messages"`
	MessageIDs []string `json:"message_ids" jsonschema:"Message IDs to mark as read"`
	SenderJID  string   `json:"sender_jid,omitempty" jsonschema:"Sender JID (required for group chats)"`
}

type manageGroupIn struct {
	Action       string   `json:"action" jsonschema:"One of create, add_members, remove_members, promote_admin, demote_admin, leave, update"`
	GroupJID     string   `json:"group_jid,omitempty" jsonschema:"Group JID (required for everything except create)"`
	Name         string   `json:"name,omitempty" jsonschema:"Group name (create, update)"`
	Participants []string `json:"participants,omitempty" jsonschema:"Participant JIDs (create, add_members, remove_members)"`
	Participant  string   `json:"participant,omitempty" jsonschema:"One participant JID (promote_admin, demote_admin)"`
	Topic        string   `json:"topic,omitempty" jsonschema:"Group topic/description (update)"`
}

type createPollIn struct {
	ChatJID     string   `json:"chat_jid" jsonschema:"The JID of the chat to send the poll to"`
	Question    string   `json:"question" jsonschema:"The poll question"`
	Options     []string `json:"options" jsonschema:"2 to 12 poll options"`
	MultiSelect bool     `json:"multi_select,omitempty" jsonschema:"Allow multiple selections (default false)"`
}

type requestHistoryIn struct {
	ChatJID            string `json:"chat_jid" jsonschema:"The JID of the chat to request history for"`
	OldestMsgID        string `json:"oldest_msg_id" jsonschema:"The ID of the oldest message currently in the chat"`
	OldestMsgTimestamp int64  `json:"oldest_msg_timestamp" jsonschema:"Unix timestamp in milliseconds of the oldest message"`
	OldestMsgFromMe    bool   `json:"oldest_msg_from_me,omitempty" jsonschema:"Whether the oldest message was sent by you (default false)"`
	Count              int    `json:"count,omitempty" jsonschema:"Number of messages to request (max 50, default 50)"`
}

type setPresenceIn struct {
	Presence string `json:"presence" jsonschema:"Either \"available\" or \"unavailable\""`
}

type jidIn struct {
	JID string `json:"jid" jsonschema:"The JID of the contact (e.g. 123456789@s.whatsapp.net)"`
}

type profilePictureIn struct {
	JID     string `json:"jid" jsonschema:"The JID of the user or group"`
	Preview bool   `json:"preview,omitempty" jsonschema:"If true, get the thumbnail instead of full resolution (default false)"`
}

type manageBlocklistIn struct {
	Action string `json:"action" jsonschema:"block or unblock"`
	JID    string `json:"jid" jsonschema:"The JID of the user"`
}

type manageNewsletterIn struct {
	Action      string `json:"action" jsonschema:"follow, unfollow or create"`
	JID         string `json:"jid,omitempty" jsonschema:"Newsletter JID (follow, unfollow)"`
	Name        string `json:"name,omitempty" jsonschema:"Newsletter name (create)"`
	Description string `json:"description,omitempty" jsonschema:"Newsletter description (create)"`
}

func (s *Server) registerWriteTools() {
	tool(s, "send_message", "Send Message",
		"Send a WhatsApp message to a person or group. For group chats use the JID. Returns success, message_id, timestamp, recipient.",
		writes(), func(_ context.Context, in sendMessageIn) (any, error) {
			if in.Recipient == "" {
				return fail("Recipient must be provided"), nil
			}
			r := s.client.SendMessage(s.store, in.Recipient, in.Message, "", in.MentionedJIDs)
			return sendOut(in.Recipient, r.Success, r.MessageID, r.Timestamp.Format(timeLayout), r.Error), nil
		})

	tool(s, "send_reaction", "Send Reaction", "Send an emoji reaction to a WhatsApp message (empty emoji removes your reaction).",
		writes(), func(_ context.Context, in sendReactionIn) (any, error) {
			action := "add"
			if in.Emoji == "" {
				action = "remove"
			}
			out := m{"success": true, "chat_jid": in.ChatJID, "message_id": in.MessageID, "emoji": in.Emoji, "action": action}
			if err := s.client.SendReaction(s.store, in.ChatJID, in.MessageID, in.Emoji); err != nil {
				out["success"], out["error"] = false, err.Error()
			}
			return out, nil
		})

	tool(s, "edit_message", "Edit Message", "Edit a previously sent WhatsApp message (your own).",
		writes(), func(_ context.Context, in editMessageIn) (any, error) {
			out := m{"success": true, "chat_jid": in.ChatJID, "message_id": in.MessageID, "new_content": in.NewContent}
			if err := s.client.EditMessage(in.ChatJID, in.MessageID, in.NewContent); err != nil {
				out["success"], out["error"] = false, err.Error()
			}
			return out, nil
		})

	tool(s, "delete_message", "Delete Message", "Delete (revoke) a WhatsApp message. sender_jid lets a group admin revoke someone else's message.",
		destructive(), func(_ context.Context, in deleteMessageIn) (any, error) {
			out := m{"success": true, "chat_jid": in.ChatJID, "message_id": in.MessageID}
			if err := s.client.DeleteMessage(in.ChatJID, in.MessageID, in.SenderJID); err != nil {
				out["success"], out["error"] = false, err.Error()
			}
			return out, nil
		})

	tool(s, "get_group_info", "Get Group Info", "Get a WhatsApp group's name, topic and participants (with admin flags and contact names where known).",
		readOnly(), func(_ context.Context, in groupJIDIn) (any, error) {
			info, err := s.client.GetGroupInfo(in.GroupJID)
			if err != nil {
				return m{"success": false, "group_jid": in.GroupJID, "error": err.Error()}, nil
			}
			participants := []m{}
			for _, p := range info.Participants {
				var name any
				if c, _ := s.contactByJID(p.JID.String()); c != nil && c.Name != nil {
					name = *c.Name
				}
				participants = append(participants, m{"jid": p.JID.String(), "name": name, "is_admin": p.IsAdmin, "is_super_admin": p.IsSuperAdmin})
			}
			return m{
				"success": true, "group_jid": in.GroupJID, "name": info.Name, "topic": info.Topic,
				"created_at": info.GroupCreated.Format(timeLayout), "created_by": info.OwnerJID.String(),
				"participant_count": len(participants), "participants": participants,
			}, nil
		})

	tool(s, "mark_read", "Mark Read", "Mark WhatsApp messages as read (sends blue ticks). sender_jid is required for group chats.",
		writes(), func(_ context.Context, in markReadIn) (any, error) {
			out := m{"success": true, "chat_jid": in.ChatJID, "message_ids": in.MessageIDs, "count": len(in.MessageIDs)}
			if err := s.client.MarkMessagesRead(in.ChatJID, in.MessageIDs, in.SenderJID); err != nil {
				out["success"], out["error"] = false, err.Error()
			}
			return out, nil
		})

	tool(s, "manage_group", "Manage Group",
		"Manage WhatsApp groups with one composable tool. Use action='create', 'add_members', 'remove_members', 'promote_admin', 'demote_admin', 'leave', or 'update'. Requires group admin rights for most actions.",
		destructive(), func(_ context.Context, in manageGroupIn) (any, error) {
			return s.manageGroup(in), nil
		})

	tool(s, "create_poll", "Create Poll", "Create and send a poll (2–12 options) to a WhatsApp chat.",
		writes(), func(_ context.Context, in createPollIn) (any, error) {
			if in.ChatJID == "" || in.Question == "" || len(in.Options) < 2 {
				return fail("chat_jid, question, and at least 2 options are required"), nil
			}
			if len(in.Options) > 12 {
				return fail("Maximum 12 options allowed"), nil
			}
			r, err := s.client.CreatePoll(in.ChatJID, in.Question, in.Options, in.MultiSelect)
			if err != nil {
				return fail("Failed to create poll: %v", err), nil
			}
			return m{"success": r.Success, "message_id": r.MessageID, "timestamp": r.Timestamp.Format(timeLayout), "chat_jid": in.ChatJID, "question": in.Question, "options": in.Options}, nil
		})

	tool(s, "request_history", "Request History",
		"Request older messages for a chat from the phone (on-demand history sync). Get oldest_msg_id and oldest_msg_timestamp from list_messages; wait a few seconds, then list_messages again.",
		writes(), func(_ context.Context, in requestHistoryIn) (any, error) {
			count := in.Count
			if count <= 0 || count > 50 {
				count = 50
			}
			if in.ChatJID == "" || in.OldestMsgID == "" || in.OldestMsgTimestamp == 0 {
				return m{"success": false, "chat_jid": in.ChatJID, "error": "chat_jid, oldest_msg_id, and oldest_msg_timestamp are required"}, nil
			}
			if err := s.client.RequestChatHistory(in.ChatJID, in.OldestMsgID, in.OldestMsgFromMe, "", in.OldestMsgTimestamp, count); err != nil {
				return m{"success": false, "chat_jid": in.ChatJID, "error": fmt.Sprintf("Failed to request history: %v", err)}, nil
			}
			return m{"success": true, "message": "History request sent. Messages will arrive via HistorySync event.", "chat_jid": in.ChatJID, "count": count, "error": nil}, nil
		})

	tool(s, "set_presence", "Set Presence", "Set your own presence status: \"available\" or \"unavailable\".",
		writes(), func(_ context.Context, in setPresenceIn) (any, error) {
			if in.Presence != "available" && in.Presence != "unavailable" {
				return fail("presence must be 'available' or 'unavailable'"), nil
			}
			if err := s.client.SetPresence(in.Presence); err != nil {
				return fail("Failed to set presence: %v", err), nil
			}
			return m{"success": true, "presence": in.Presence}, nil
		})

	tool(s, "subscribe_presence", "Subscribe Presence", "Subscribe to presence updates for a contact.",
		writes(), func(_ context.Context, in jidIn) (any, error) {
			if err := s.client.SubscribeToPresence(in.JID); err != nil {
				return fail("Failed to subscribe to presence: %v", err), nil
			}
			return m{"success": true, "jid": in.JID}, nil
		})

	tool(s, "get_profile_picture", "Get Profile Picture", "Get the profile picture URL for a user or group (has_picture=false if none).",
		readOnly(), func(_ context.Context, in profilePictureIn) (any, error) {
			info, err := s.client.GetProfilePicture(in.JID, in.Preview)
			if err != nil {
				return fail("Failed to get profile picture: %v", err), nil
			}
			if info == nil {
				return m{"success": true, "jid": in.JID, "has_picture": false}, nil
			}
			return m{"success": true, "jid": in.JID, "has_picture": true, "url": info.URL, "id": info.ID, "type": info.Type, "direct_path": info.DirectPath}, nil
		})

	tool(s, "get_blocklist", "Get Blocklist", "Get the list of blocked WhatsApp users.",
		readOnly(), func(_ context.Context, _ struct{}) (any, error) {
			users, err := s.client.GetBlockedUsers()
			if err != nil {
				return fail("Failed to get blocklist: %v", err), nil
			}
			if users == nil {
				users = []localTypes.BlockedUser{}
			}
			return m{"success": true, "users": users, "count": len(users)}, nil
		})

	tool(s, "manage_blocklist", "Manage Blocklist", "Block or unblock WhatsApp users. Use get_blocklist for the read-only listing.",
		destructive(), func(_ context.Context, in manageBlocklistIn) (any, error) {
			if in.Action != "block" && in.Action != "unblock" {
				return invalidAction(in.Action, []string{"block", "unblock"}, "manage_blocklist"), nil
			}
			if in.JID == "" {
				return m{"success": false, "error": "jid is required for block/unblock", "use_tool": "manage_blocklist"}, nil
			}
			if err := s.client.UpdateBlockedUser(in.JID, in.Action); err != nil {
				return m{"success": false, "jid": in.JID, "action": in.Action, "error": err.Error()}, nil
			}
			return m{"success": true, "jid": in.JID, "action": in.Action}, nil
		})

	tool(s, "manage_newsletter", "Manage Newsletter", "Follow, unfollow, or create WhatsApp newsletters/channels.",
		destructive(), func(_ context.Context, in manageNewsletterIn) (any, error) {
			switch in.Action {
			case "create":
				if in.Name == "" {
					return m{"success": false, "error": "name is required for action='create'", "use_tool": "manage_newsletter"}, nil
				}
				info, err := s.client.CreateNewsletterChannel(in.Name, in.Description)
				if err != nil {
					return fail("Failed to create newsletter: %v", err), nil
				}
				return m{"success": true, "jid": info.JID, "name": info.Name, "description": info.Description}, nil
			case "follow", "unfollow":
				if in.JID == "" {
					return m{"success": false, "error": "jid is required for follow/unfollow", "use_tool": "manage_newsletter"}, nil
				}
				var err error
				if in.Action == "follow" {
					err = s.client.FollowNewsletterChannel(in.JID)
				} else {
					err = s.client.UnfollowNewsletterChannel(in.JID)
				}
				if err != nil {
					return m{"success": false, "jid": in.JID, "action": in.Action, "error": err.Error()}, nil
				}
				return m{"success": true, "jid": in.JID, "action": in.Action}, nil
			default:
				return invalidAction(in.Action, []string{"follow", "unfollow", "create"}, "manage_newsletter"), nil
			}
		})
}

const timeLayout = "2006-01-02T15:04:05Z07:00"

func sendOut(recipient string, success bool, messageID, timestamp, errText string) m {
	out := m{"success": success, "message_id": messageID, "timestamp": timestamp, "recipient": recipient, "error": nil}
	if !success {
		out["message_id"], out["timestamp"] = nil, nil
		out["error"] = errText
	}
	return out
}

func participantsOut(ps []types.GroupParticipant) []m {
	out := []m{}
	for _, p := range ps {
		entry := m{"jid": p.JID.String()}
		if p.Error != 0 {
			entry["error"] = p.Error
		}
		out = append(out, entry)
	}
	return out
}

func (s *Server) manageGroup(in manageGroupIn) m {
	allowed := []string{"create", "add_members", "remove_members", "promote_admin", "demote_admin", "leave", "update"}
	need := func(msg string) m {
		return m{"success": false, "error": msg, "use_tool": "manage_group"}
	}
	switch in.Action {
	case "create":
		if in.Name == "" || len(in.Participants) == 0 {
			return need("name and participants are required for action='create'")
		}
		info, err := s.client.CreateGroup(in.Name, in.Participants)
		if err != nil {
			return fail("Failed to create group: %v", err)
		}
		return m{"success": true, "group_jid": info.JID.String(), "name": info.Name}
	case "add_members", "remove_members", "promote_admin", "demote_admin", "leave", "update":
		if in.GroupJID == "" {
			return need("group_jid is required for this action")
		}
	default:
		return invalidAction(in.Action, allowed, "manage_group")
	}
	switch in.Action {
	case "add_members", "remove_members":
		if len(in.Participants) == 0 {
			return need(fmt.Sprintf("participants is required for action='%s'", in.Action))
		}
		var ps []types.GroupParticipant
		var err error
		if in.Action == "add_members" {
			ps, err = s.client.AddGroupParticipants(in.GroupJID, in.Participants)
		} else {
			ps, err = s.client.RemoveGroupParticipants(in.GroupJID, in.Participants)
		}
		if err != nil {
			return m{"success": false, "group_jid": in.GroupJID, "error": err.Error()}
		}
		return m{"success": true, "group_jid": in.GroupJID, "participants": participantsOut(ps)}
	case "promote_admin", "demote_admin":
		if in.Participant == "" {
			return need(fmt.Sprintf("participant is required for action='%s'", in.Action))
		}
		var err error
		action := "promoted"
		if in.Action == "promote_admin" {
			_, err = s.client.PromoteGroupParticipant(in.GroupJID, in.Participant)
		} else {
			action = "demoted"
			_, err = s.client.DemoteGroupParticipant(in.GroupJID, in.Participant)
		}
		if err != nil {
			return m{"success": false, "group_jid": in.GroupJID, "participant": in.Participant, "error": err.Error()}
		}
		return m{"success": true, "group_jid": in.GroupJID, "participant": in.Participant, "action": action}
	case "leave":
		if err := s.client.LeaveGroup(in.GroupJID); err != nil {
			return m{"success": false, "group_jid": in.GroupJID, "error": err.Error()}
		}
		return m{"success": true, "group_jid": in.GroupJID, "action": "left"}
	default: // update
		if in.Name == "" && in.Topic == "" {
			return need("name or topic is required for action='update'")
		}
		var errs []string
		if in.Name != "" {
			if err := s.client.SetGroupName(in.GroupJID, in.Name); err != nil {
				errs = append(errs, "name: "+err.Error())
			}
		}
		if in.Topic != "" {
			if err := s.client.SetGroupTopic(in.GroupJID, in.Topic); err != nil {
				errs = append(errs, "topic: "+err.Error())
			}
		}
		if len(errs) > 0 {
			return m{"success": false, "group_jid": in.GroupJID, "error": "Partial failure: " + strings.Join(errs, "; ")}
		}
		return m{"success": true, "group_jid": in.GroupJID}
	}
}
