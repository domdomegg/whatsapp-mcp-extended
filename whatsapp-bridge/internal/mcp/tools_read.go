package mcp

import (
	"context"
	"fmt"

	"whatsapp-bridge/internal/database"
)

type searchContactsIn struct {
	Query string `json:"query" jsonschema:"Search term to match against contact names or phone numbers"`
}

type listMessagesIn struct {
	After          string `json:"after,omitempty" jsonschema:"Optional ISO-8601 formatted string to only return messages after this date"`
	Before         string `json:"before,omitempty" jsonschema:"Optional ISO-8601 formatted string to only return messages before this date"`
	ChatJID        string `json:"chat_jid,omitempty" jsonschema:"Optional chat JID to filter messages by chat"`
	Query          string `json:"query,omitempty" jsonschema:"Optional search term to filter messages by content"`
	Limit          int    `json:"limit,omitempty" jsonschema:"Maximum number of messages to return (default 20)"`
	Page           int    `json:"page,omitempty" jsonschema:"Page number for pagination (default 0)"`
	IncludeContext bool   `json:"include_context,omitempty" jsonschema:"Return surrounding messages inline for each result (default false)"`
	ContextBefore  *int   `json:"context_before,omitempty" jsonschema:"Messages before each result to include (default 1, requires include_context)"`
	ContextAfter   *int   `json:"context_after,omitempty" jsonschema:"Messages after each result to include (default 1, requires include_context)"`
}

type listChatsIn struct {
	Query              string `json:"query,omitempty" jsonschema:"Optional search term to filter chats by name or JID"`
	Limit              int    `json:"limit,omitempty" jsonschema:"Maximum number of chats to return (default 20)"`
	Page               int    `json:"page,omitempty" jsonschema:"Page number for pagination (default 0)"`
	IncludeLastMessage *bool  `json:"include_last_message,omitempty" jsonschema:"Whether to include the last message in each chat (default true)"`
	SortBy             string `json:"sort_by,omitempty" jsonschema:"Field to sort results by, either \"last_active\" or \"name\" (default \"last_active\")"`
}

type getChatIn struct {
	ChatJID            string `json:"chat_jid" jsonschema:"The JID of the chat to retrieve"`
	IncludeLastMessage *bool  `json:"include_last_message,omitempty" jsonschema:"Whether to include the last message (default true)"`
}

type getMessageContextIn struct {
	MessageID string `json:"message_id" jsonschema:"The ID of the message to get context for"`
	Before    *int   `json:"before,omitempty" jsonschema:"Number of messages to include before the target message (default 5)"`
	After     *int   `json:"after,omitempty" jsonschema:"Number of messages to include after the target message (default 5)"`
}

type listAllContactsIn struct {
	Limit int `json:"limit,omitempty" jsonschema:"Maximum number of contacts to return (default 100)"`
}

type getContactContextIn struct {
	Identifier             string `json:"identifier" jsonschema:"Contact JID or phone number"`
	IncludeChats           bool   `json:"include_chats,omitempty" jsonschema:"Also return the chats this contact appears in and their direct chat"`
	IncludeLastInteraction bool   `json:"include_last_interaction,omitempty" jsonschema:"Also return the most recent message with this contact"`
	Limit                  int    `json:"limit,omitempty" jsonschema:"Maximum number of chats to return (default 20)"`
	Page                   int    `json:"page,omitempty" jsonschema:"Page number for pagination (default 0)"`
}

type getDirectChatIn struct {
	PhoneNumber string `json:"phone_number" jsonschema:"Phone number to look up — digits only, partial match supported (e.g. \"60123456789\")"`
}

type manageNicknameIn struct {
	Action   string `json:"action" jsonschema:"One of set, get, remove, list"`
	JID      string `json:"jid,omitempty" jsonschema:"Contact JID (required for set/get/remove)"`
	Nickname string `json:"nickname,omitempty" jsonschema:"Nickname to set (required for set)"`
}

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func orDefaultPtr(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}

func orTrue(v *bool) bool { return v == nil || *v }

func (s *Server) registerReadTools() {
	tool(s, "search_contacts", "Search Contacts",
		"Search WhatsApp contacts by name or phone number. Returns contact dicts with jid, phone_number, name, first_name, full_name, push_name, business_name, nickname. Use the returned jid with list_messages to filter by contact; use get_contact_context for details, chats or last interaction.",
		readOnly(), func(_ context.Context, in searchContactsIn) (any, error) {
			return s.searchContacts(in.Query)
		})

	tool(s, "list_messages", "List Messages",
		"Get WhatsApp messages matching the criteria, newest first. If expected messages are missing, use request_history to sync older messages from the phone. Set include_context=true to get surrounding messages inline. Use search_contacts first to find a contact's JID.",
		readOnly(), func(_ context.Context, in listMessagesIn) (any, error) {
			o := database.ListMessagesOptions{
				ChatJID: in.ChatJID, Query: in.Query, Limit: orDefault(in.Limit, 20), Page: in.Page,
				IncludeContext: in.IncludeContext, ContextBefore: orDefaultPtr(in.ContextBefore, 1), ContextAfter: orDefaultPtr(in.ContextAfter, 1),
			}
			if in.After != "" {
				t, err := database.ParseTime(in.After)
				if err != nil {
					return nil, fmt.Errorf("invalid date format for 'after': %s", in.After)
				}
				o.After = &t
			}
			if in.Before != "" {
				t, err := database.ParseTime(in.Before)
				if err != nil {
					return nil, fmt.Errorf("invalid date format for 'before': %s", in.Before)
				}
				o.Before = &t
			}
			return s.store.ListMessages(o)
		})

	tool(s, "list_chats", "List Chats",
		"Get WhatsApp chats matching the criteria. Use the returned jid with list_messages for a chat's messages, get_chat for one chat's metadata, and get_group_info (jid ending @g.us) for a group's participants.",
		readOnly(), func(_ context.Context, in listChatsIn) (any, error) {
			return s.store.ListChats(database.ListChatsOptions{
				Query: in.Query, Limit: orDefault(in.Limit, 20), Page: in.Page,
				IncludeLastMessage: orTrue(in.IncludeLastMessage), SortByName: in.SortBy == "name",
			})
		})

	tool(s, "get_chat", "Get Chat", "Get WhatsApp chat metadata by JID.",
		readOnly(), func(_ context.Context, in getChatIn) (any, error) {
			c, err := s.store.GetChat(in.ChatJID, orTrue(in.IncludeLastMessage))
			if err != nil {
				return nil, err
			}
			if c == nil {
				return nil, nil
			}
			return c, nil
		})

	tool(s, "get_message_context", "Get Message Context",
		"Get the messages around a specific WhatsApp message (get message_id from list_messages first).",
		readOnly(), func(_ context.Context, in getMessageContextIn) (any, error) {
			return s.store.GetMessageContext(in.MessageID, orDefaultPtr(in.Before, 5), orDefaultPtr(in.After, 5))
		})

	tool(s, "list_all_contacts", "List All Contacts",
		"List WhatsApp contacts with jid, phone_number, name, first_name, full_name, push_name, business_name, nickname.",
		readOnly(), func(_ context.Context, in listAllContactsIn) (any, error) {
			rows, err := s.contacts.ListAll(orDefault(in.Limit, 100))
			if err != nil {
				return nil, err
			}
			out := []database.ContactOut{}
			for _, r := range rows {
				c := database.ContactFromRow(r, "")
				if c.Name == nil {
					// list_all_contacts also fell back to business name, then phone.
					name := r.BusinessName
					if name == "" {
						name = c.PhoneNumber
					}
					c.Name = &name
				}
				out = append(out, c)
			}
			return out, nil
		})

	tool(s, "get_contact_context", "Get Contact Context",
		"Get contact details plus optional related chats and last interaction. Use after search_contacts when you need more context for a person.",
		readOnly(), func(_ context.Context, in getContactContextIn) (any, error) {
			contact, err := s.contactByJID(in.Identifier)
			if err != nil {
				return nil, err
			}
			if contact == nil {
				if contact, err = s.contactByPhone(in.Identifier); err != nil {
					return nil, err
				}
			}
			jid := in.Identifier
			if contact != nil {
				jid = contact.JID
			}
			out := m{"contact": contact}
			if in.IncludeChats {
				chats, err := s.store.GetContactChats(jid, orDefault(in.Limit, 20), in.Page)
				if err != nil {
					return nil, err
				}
				out["chats"] = chats
				direct, err := s.store.GetDirectChatByContact(database.PhoneFromJID(jid))
				if err != nil {
					return nil, err
				}
				out["direct_chat"] = direct
			}
			if in.IncludeLastInteraction {
				last, err := s.store.GetLastInteraction(jid)
				if err != nil {
					return nil, err
				}
				out["last_interaction"] = last
			}
			return out, nil
		})

	tool(s, "get_direct_chat_by_contact", "Get Direct Chat By Contact",
		"Find the direct-message chat for a phone number (DMs only, partial match). Faster than get_contact_context when you only need the chat JID.",
		readOnly(), func(_ context.Context, in getDirectChatIn) (any, error) {
			c, err := s.store.GetDirectChatByContact(in.PhoneNumber)
			if err != nil {
				return nil, err
			}
			if c == nil {
				return nil, nil
			}
			return c, nil
		})

	tool(s, "manage_nickname", "Manage Nickname",
		"Manage custom contact nicknames. Use action='set', 'get', 'remove', or 'list'. This changes only local nickname metadata.",
		writes(), func(_ context.Context, in manageNicknameIn) (any, error) {
			allowed := []string{"set", "get", "remove", "list"}
			switch in.Action {
			case "list":
				all, err := s.store.ListNicknames()
				if err != nil {
					return nil, err
				}
				return m{"success": true, "nicknames": all}, nil
			case "set", "get", "remove":
			default:
				return invalidAction(in.Action, allowed, "manage_nickname"), nil
			}
			if in.JID == "" {
				return m{"success": false, "error": "jid is required for set/get/remove", "use_tool": "manage_nickname"}, nil
			}
			switch in.Action {
			case "set":
				if in.Nickname == "" {
					return m{"success": false, "error": "nickname is required for action='set'", "use_tool": "manage_nickname"}, nil
				}
				if err := s.store.SetNickname(in.JID, in.Nickname); err != nil {
					return nil, err
				}
				return m{"success": true, "jid": in.JID, "nickname": in.Nickname}, nil
			case "get":
				n, err := s.store.GetNickname(in.JID)
				if err != nil {
					return nil, err
				}
				var nick any
				if n != "" {
					nick = n
				}
				return m{"success": true, "jid": in.JID, "nickname": nick}, nil
			default:
				found, err := s.store.RemoveNickname(in.JID)
				if err != nil {
					return nil, err
				}
				if !found {
					return m{"success": false, "jid": in.JID, "error": "No nickname found"}, nil
				}
				return m{"success": true, "jid": in.JID}, nil
			}
		})
}

func (s *Server) searchContacts(query string) ([]database.ContactOut, error) {
	rows, err := s.contacts.Search(query)
	if err != nil {
		return nil, err
	}
	out := []database.ContactOut{}
	for _, r := range rows {
		nick, err := s.store.GetNickname(r.JID)
		if err != nil {
			return nil, err
		}
		out = append(out, database.ContactFromRow(r, nick))
	}
	return out, nil
}

func (s *Server) contactByJID(jid string) (*database.ContactOut, error) {
	row, err := s.contacts.Get(jid)
	if err != nil || row == nil {
		return nil, err
	}
	nick, err := s.store.GetNickname(jid)
	if err != nil {
		return nil, err
	}
	c := database.ContactFromRow(*row, nick)
	return &c, nil
}

// contactByPhone tries the JID forms a phone number can take, then falls back
// to any non-group chat whose JID contains the number.
func (s *Server) contactByPhone(phone string) (*database.ContactOut, error) {
	for _, jid := range []string{phone + "@s.whatsapp.net", phone + "@c.us", phone} {
		c, err := s.contactByJID(jid)
		if err != nil || c != nil {
			return c, err
		}
	}
	chat, err := s.store.GetDirectChatByContact(phone)
	if err != nil || chat == nil {
		return nil, err
	}
	actual := database.PhoneFromJID(chat.JID)
	name := actual
	if chat.Name != nil && *chat.Name != "" {
		name = *chat.Name
	}
	return &database.ContactOut{JID: chat.JID, PhoneNumber: actual, Name: &name}, nil
}
