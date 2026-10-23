package types

import (
	"time"
)

// ContactInfo represents condensed contact information embedded in messages/chats
type ContactInfo struct {
	JID       string `json:"jid,omitempty"`
	Name      string `json:"name,omitempty"`
	PhoneNum  string `json:"phone_number,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	Nickname  string `json:"nickname,omitempty"`
}

// MessageVelocity represents message frequency metrics
type MessageVelocity struct {
	MessagesPerDay float64 `json:"messages_per_day,omitempty"`
	TrendDirection string  `json:"trend_direction,omitempty"` // "increasing", "stable", "decreasing"
}

// ActivityTrend represents activity pattern information
type ActivityTrend struct {
	Direction     string  `json:"direction,omitempty"` // "increasing", "stable", "decreasing"
	AveragePerDay float64 `json:"average_per_day,omitempty"`
	PeakHourRange string  `json:"peak_hour_range,omitempty"` // "HH:MM-HH:MM" or empty
	MostActiveDay string  `json:"most_active_day,omitempty"` // day of week or empty
}

// Message represents a chat message for our client
type Message struct {
	ID                      string         `json:"id,omitempty"`
	Time                    time.Time      `json:"timestamp,omitempty"`
	Sender                  string         `json:"sender,omitempty"`
	SenderName              string         `json:"sender_name,omitempty"`
	SenderContactInfo       *ContactInfo   `json:"sender_contact_info,omitempty"`
	Content                 string         `json:"content,omitempty"`
	IsFromMe                bool           `json:"is_from_me,omitempty"`
	MediaType               string         `json:"media_type,omitempty"`
	Filename                string         `json:"filename,omitempty"`
	CharacterCount          int            `json:"character_count,omitempty"`
	WordCount               int            `json:"word_count,omitempty"`
	URLList                 []string       `json:"url_list,omitempty"`
	Mentions                []string       `json:"mentions,omitempty"`
	ReactionSummary         map[string]int `json:"reaction_summary,omitempty"`
	QuotedMessageID         string         `json:"quoted_message_id,omitempty"`
	QuotedSenderName        string         `json:"quoted_sender_name,omitempty"`
	ReplyToMessageID        string         `json:"reply_to_message_id,omitempty"`
	EditCount               int            `json:"edit_count,omitempty"`
	IsEdited                bool           `json:"is_edited,omitempty"`
	IsForwarded             bool           `json:"is_forwarded,omitempty"`
	ForwardedFrom           string         `json:"forwarded_from,omitempty"`
	IsSystemMessage         bool           `json:"is_system_message,omitempty"`
	SystemMessageType       string         `json:"system_message_type,omitempty"`
	ResponseTimeSeconds     float64        `json:"response_time_seconds,omitempty"`
	IsFirstMessageToday     bool           `json:"is_first_message_today,omitempty"`
	HasReactions            bool           `json:"has_reactions,omitempty"`
	MessagePositionInThread int            `json:"message_position_in_thread,omitempty"`
	IsGroup                 bool           `json:"is_group,omitempty"`
	IsRead                  bool           `json:"is_read,omitempty"`
}

// Chat represents a WhatsApp chat/conversation
type Chat struct {
	JID                      string           `json:"jid,omitempty"`
	Name                     string           `json:"name,omitempty"`
	IsGroup                  bool             `json:"is_group,omitempty"`
	LastMessageTime          time.Time        `json:"last_message_time,omitempty"`
	LastMessage              string           `json:"last_message,omitempty"`
	LastMessageID            string           `json:"last_message_id,omitempty"`
	LastSender               string           `json:"last_sender,omitempty"`
	LastSenderName           string           `json:"last_sender_name,omitempty"`
	LastSenderContactInfo    *ContactInfo     `json:"last_sender_contact_info,omitempty"`
	LastIsFromMe             bool             `json:"last_is_from_me,omitempty"`
	TotalMessageCount        int              `json:"total_message_count,omitempty"`
	MessageCountToday        int              `json:"message_count_today,omitempty"`
	MessageCountLast7Days    int              `json:"message_count_last_7_days,omitempty"`
	MessageVelocityLast7Days *MessageVelocity `json:"message_velocity_last_7_days,omitempty"`
	ParticipantCount         int              `json:"participant_count,omitempty"`
	ParticipantNames         []string         `json:"participant_names,omitempty"`
	ParticipantList          []*ContactInfo   `json:"participant_list,omitempty"`
	MostActiveMemberName     string           `json:"most_active_member_name,omitempty"`
	MostActiveMemberMsgCount int              `json:"most_active_member_message_count,omitempty"`
	AdminList                []string         `json:"admin_list,omitempty"`
	ChatType                 string           `json:"chat_type,omitempty"` // "individual", "group", "broadcast"
	SilentDurationSeconds    int              `json:"silent_duration_seconds,omitempty"`
	IsRecentlyActive         bool             `json:"is_recently_active,omitempty"`
	MediaCountByType         map[string]int   `json:"media_count_by_type,omitempty"`
	RecentMedia              []string         `json:"recent_media,omitempty"`
	HasMedia                 bool             `json:"has_media,omitempty"`
	IsDisappearingMessages   bool             `json:"is_disappearing_messages,omitempty"`
	DisappearingTTL          string           `json:"disappearing_ttl,omitempty"`
	Timezone                 string           `json:"timezone,omitempty"`
	LastMessageTimeAgo       string           `json:"last_message_time_ago,omitempty"`
}

// Contact represents a WhatsApp contact
type Contact struct {
	JID                        string         `json:"jid,omitempty"`
	PhoneNumber                string         `json:"phone_number,omitempty"`
	Name                       string         `json:"name,omitempty"`
	FirstName                  string         `json:"first_name,omitempty"`
	FullName                   string         `json:"full_name,omitempty"`
	PushName                   string         `json:"push_name,omitempty"`
	BusinessName               string         `json:"business_name,omitempty"`
	Nickname                   string         `json:"nickname,omitempty"`
	RelationshipType           string         `json:"relationship_type,omitempty"` // "friend", "colleague", "family", "other"
	IsFavorite                 bool           `json:"is_favorite,omitempty"`
	ContactCreatedDate         time.Time      `json:"contact_created_date,omitempty"`
	SharedGroupList            []*ContactInfo `json:"shared_group_list,omitempty"`
	SharedGroupCount           int            `json:"shared_group_count,omitempty"`
	TotalMessageCount          int            `json:"total_message_count,omitempty"`
	MessageCountToday          int            `json:"message_count_today,omitempty"`
	MessageCountLast7Days      int            `json:"message_count_last_7_days,omitempty"`
	MessageCountLast30Days     int            `json:"message_count_last_30_days,omitempty"`
	ActivityTrend              *ActivityTrend `json:"activity_trend,omitempty"`
	TypicalResponseTimeSeconds float64        `json:"typical_response_time_seconds,omitempty"`
	TypicalReplyRate           float64        `json:"typical_reply_rate,omitempty"`
	IsResponsive               bool           `json:"is_responsive,omitempty"`
	DaysSinceLastMessage       int            `json:"days_since_last_message,omitempty"`
	LastSeenTimestamp          time.Time      `json:"last_seen_timestamp,omitempty"`
	Timezone                   string         `json:"timezone,omitempty"`
	Organization               string         `json:"organization,omitempty"`
	StatusMessage              string         `json:"status_message,omitempty"`
	LatestMessagePreview       string         `json:"latest_message_preview,omitempty"`
	LatestMessageTimestamp     time.Time      `json:"latest_message_timestamp,omitempty"`
	HasActiveChat              bool           `json:"has_active_chat,omitempty"`
	RecentChatJID              string         `json:"recent_chat_jid,omitempty"`
}

type GroupInfo struct {
	IsGroup          bool   `json:"is_group"`
	GroupName        string `json:"group_name"`
	ParticipantCount int    `json:"participant_count"`
}

// SendResult contains the result of sending a message (internal use)
type SendResult struct {
	Success   bool
	Error     string
	MessageID string
	Timestamp time.Time
}

// ProfilePictureInfo represents profile picture information
type ProfilePictureInfo struct {
	URL        string `json:"url"`
	ID         string `json:"id"`
	Type       string `json:"type"`
	DirectPath string `json:"direct_path,omitempty"`
}

// BlockedUser represents a blocked user
type BlockedUser struct {
	JID string `json:"jid"`
}

// NewsletterInfo represents newsletter metadata
type NewsletterInfo struct {
	JID         string `json:"jid"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}
