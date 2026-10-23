package database

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *MessageStore {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/messages.db?_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := createTables(db); err != nil {
		t.Fatal(err)
	}
	return &MessageStore{db: db}
}

func seed(t *testing.T, store *MessageStore) {
	t.Helper()
	base := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(store.StoreChat("111@s.whatsapp.net", "Alice", base.Add(3*time.Minute)))
	must(store.StoreChat("222@g.us", "Group", base.Add(time.Minute)))
	must(store.StoreMessage("m1", "111@s.whatsapp.net", "111@s.whatsapp.net", "Alice", "hi there https://a.example", base, false, "", "", "", "", nil, nil, nil, 0))
	must(store.StoreMessage("m2", "111@s.whatsapp.net", "me", "", "hello back", base.Add(time.Minute), true, "", "", "", "", nil, nil, nil, 0))
	must(store.StoreMessage("m3", "111@s.whatsapp.net", "111@s.whatsapp.net", "Alice", "photo", base.Add(3*time.Minute), false, "image", "pic.jpg", "u", "d", nil, nil, nil, 123))
	must(store.StoreMessage("g1", "222@g.us", "333@s.whatsapp.net", "Bob", "group msg", base.Add(time.Minute), false, "", "", "", "", nil, nil, nil, 0))
}

func TestListMessagesShapesLikePython(t *testing.T) {
	store := openTestStore(t)
	seed(t, store)
	msgs, err := store.ListMessages(ListMessagesOptions{ChatJID: "111@s.whatsapp.net", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 || msgs[0].ID != "m3" {
		t.Fatalf("want 3 newest-first, got %+v", msgs)
	}
	raw, _ := json.Marshal(msgs[2])
	s := string(raw)
	for _, want := range []string{`"id":"m1"`, `"timestamp":"2026-10-20T10:00:00+00:00"`, `"is_group":false`, `"chat_name":"Alice"`, `"sender_name":"Alice"`, `"character_count":26`, `"word_count":3`, `"url_list":["https://a.example"]`, `"sender_contact_info":{"jid":"111@s.whatsapp.net","phone_number":"111"}`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
	if strings.Contains(s, "media_type") || strings.Contains(s, "is_edited") {
		t.Errorf("empty optionals should be omitted: %s", s)
	}
	raw, _ = json.Marshal(msgs[0])
	if !strings.Contains(string(raw), `"media_type":"image","filename":"pic.jpg","file_length":123`) {
		t.Errorf("media fields: %s", raw)
	}
	if msgs[1].SenderName != "me" { // falls back to the sender's user part
		t.Errorf("sender_name fallback: %q", msgs[1].SenderName)
	}
}

func TestListMessagesFiltersAndContext(t *testing.T) {
	store := openTestStore(t)
	seed(t, store)
	after := time.Date(2026, 10, 20, 10, 0, 30, 0, time.UTC)
	msgs, err := store.ListMessages(ListMessagesOptions{After: &after, Query: "HELLO", Limit: 5})
	if err != nil || len(msgs) != 1 || msgs[0].ID != "m2" {
		t.Fatalf("filter: %v %+v", err, msgs)
	}
	ctx, err := store.ListMessages(ListMessagesOptions{Query: "hello", Limit: 5, IncludeContext: true, ContextBefore: 1, ContextAfter: 1})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range ctx {
		ids = append(ids, m.ID)
	}
	if strings.Join(ids, ",") != "m1,m2,m3" {
		t.Errorf("context order: %v", ids)
	}
	if _, err := store.GetMessageContext("nope", 1, 1); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing message should be not found, got %v", err)
	}
}

func TestListChatsAndGetChat(t *testing.T) {
	store := openTestStore(t)
	seed(t, store)
	if err := store.SetNickname("111@s.whatsapp.net", "Al"); err != nil {
		t.Fatal(err)
	}
	chats, err := store.ListChats(ListChatsOptions{Limit: 10, IncludeLastMessage: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 2 || chats[0].JID != "111@s.whatsapp.net" {
		t.Fatalf("want Alice first (most recent), got %+v", chats)
	}
	a := chats[0]
	if a.LastMessage != "photo" || a.LastMessageID != "m3" || *a.TotalMessageCount != 3 || a.ChatType != "individual" || !a.HasMedia || a.MediaCountByType["image"] != 1 {
		raw, _ := json.Marshal(a)
		t.Errorf("chat shape: %s", raw)
	}
	if a.LastSenderContactInfo["nickname"] != "Al" || a.LastSenderName != "Alice" {
		t.Errorf("last sender: %+v %q", a.LastSenderContactInfo, a.LastSenderName)
	}
	raw, _ := json.Marshal(chats[1])
	if !strings.Contains(string(raw), `"name":"Group"`) || !strings.Contains(string(raw), `"is_group":true`) || !strings.Contains(string(raw), `"chat_type":"group"`) {
		t.Errorf("group chat: %s", raw)
	}
	byName, _ := store.ListChats(ListChatsOptions{Limit: 10, SortByName: true, Query: "gr"})
	if len(byName) != 1 || byName[0].JID != "222@g.us" {
		t.Errorf("query/sort: %+v", byName)
	}
	c, err := store.GetChat("111@s.whatsapp.net", true)
	if err != nil || c == nil || c.LastMessage != "photo" || c.LastMessageTime == "" {
		t.Errorf("GetChat: %v %+v", err, c)
	}
	if c, _ := store.GetChat("missing", true); c != nil {
		t.Errorf("missing chat should be nil")
	}
	d, _ := store.GetDirectChatByContact("111")
	if d == nil || d.JID != "111@s.whatsapp.net" {
		t.Errorf("direct chat: %+v", d)
	}
	cc, _ := store.GetContactChats("333@s.whatsapp.net", 10, 0)
	if len(cc) != 1 || cc[0].JID != "222@g.us" {
		t.Errorf("contact chats: %+v", cc)
	}
	li, _ := store.GetLastInteraction("111@s.whatsapp.net")
	if li == nil || li.ID != "m3" || li.CharacterCount != nil {
		t.Errorf("last interaction: %+v", li)
	}
}

func TestNicknames(t *testing.T) {
	store := openTestStore(t)
	if err := store.SetNickname("1@s.whatsapp.net", "One"); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.GetNickname("1@s.whatsapp.net"); n != "One" {
		t.Errorf("get: %q", n)
	}
	all, _ := store.ListNicknames()
	if len(all) != 1 || all[0].Nickname != "One" || all[0].CreatedAt == "" {
		t.Errorf("list: %+v", all)
	}
	if found, _ := store.RemoveNickname("1@s.whatsapp.net"); !found {
		t.Error("remove should find it")
	}
	if found, _ := store.RemoveNickname("1@s.whatsapp.net"); found {
		t.Error("second remove should not find it")
	}
}

func TestContactStore(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/whatsapp.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE whatsmeow_contacts (our_jid TEXT, their_jid TEXT, first_name TEXT, full_name TEXT, push_name TEXT, business_name TEXT);
		INSERT INTO whatsmeow_contacts VALUES ('me','111@s.whatsapp.net','Ali','Alice Smith','alice',NULL);
		INSERT INTO whatsmeow_contacts VALUES ('me','222@g.us','','','', '');
		INSERT INTO whatsmeow_contacts VALUES ('me','333@s.whatsapp.net','','','bobby','Bob Ltd')`); err != nil {
		t.Fatal(err)
	}
	cs := NewContactStoreFromDB(db)
	found, err := cs.Search("ALICE")
	if err != nil || len(found) != 1 || found[0].JID != "111@s.whatsapp.net" {
		t.Fatalf("search: %v %+v", err, found)
	}
	c := ContactFromRow(found[0], "Al")
	raw, _ := json.Marshal(c)
	for _, want := range []string{`"phone_number":"111"`, `"name":"Alice Smith"`, `"first_name":"Ali"`, `"push_name":"alice"`, `"nickname":"Al"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %s in %s", want, raw)
		}
	}
	if strings.Contains(string(raw), "business_name") {
		t.Errorf("empty business_name should be omitted: %s", raw)
	}
	all, _ := cs.ListAll(10)
	if len(all) != 2 || all[0].JID != "111@s.whatsapp.net" || all[1].JID != "333@s.whatsapp.net" {
		t.Errorf("list all (no groups, by display name): %+v", all)
	}
	if got, _ := cs.Get("nope"); got != nil {
		t.Error("Get missing should be nil")
	}
	bob := ContactFromRow(all[1], "")
	if bob.Name == nil || *bob.Name != "bobby" {
		t.Errorf("name fallback to push name: %+v", bob)
	}
}

func TestParseAndFormatTime(t *testing.T) {
	for _, in := range []string{"2026-10-20T10:00:00Z", "2026-10-20T10:00:00+00:00", "2026-10-20 10:00:00", "2026-10-20"} {
		if _, err := ParseTime(in); err != nil {
			t.Errorf("ParseTime(%q): %v", in, err)
		}
	}
	if _, err := ParseTime("yesterday"); err == nil {
		t.Error("garbage should fail")
	}
	if got := formatTime(time.Date(2026, 1, 2, 3, 4, 5, 600000000, time.UTC)); got != "2026-01-02T03:04:05.6+00:00" {
		t.Errorf("fractional: %q", got)
	}
}
