CREATE TABLE contacts (
  jid TEXT PRIMARY KEY,
  push_name TEXT,
  business_name TEXT,
  phone TEXT,
  is_blocked INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL
);

CREATE TABLE groups (
  jid TEXT PRIMARY KEY,
  name TEXT,
  topic TEXT,
  owner_jid TEXT REFERENCES contacts(jid),
  created_at INTEGER,
  updated_at INTEGER NOT NULL
);

CREATE TABLE group_participants (
  group_jid TEXT NOT NULL REFERENCES groups(jid) ON DELETE CASCADE,
  contact_jid TEXT NOT NULL REFERENCES contacts(jid),
  is_admin INTEGER NOT NULL DEFAULT 0,
  joined_at INTEGER,
  PRIMARY KEY (group_jid, contact_jid)
);

CREATE TABLE chats (
  jid TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('dm','group')),
  last_message_at INTEGER,
  unread_count INTEGER NOT NULL DEFAULT 0,
  archived INTEGER NOT NULL DEFAULT 0,
  pinned INTEGER NOT NULL DEFAULT 0,
  muted_until INTEGER
);

CREATE TABLE messages (
  id TEXT NOT NULL,
  chat_jid TEXT NOT NULL REFERENCES chats(jid),
  sender_jid TEXT NOT NULL REFERENCES contacts(jid),
  from_me INTEGER NOT NULL,
  timestamp INTEGER NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('text','image','video','audio','voice','document','sticker','location','contact','system')),
  text TEXT,
  quoted_id TEXT,
  reactions TEXT,        -- JSON array
  edited_at INTEGER,
  deleted_at INTEGER,
  raw TEXT,              -- JSON of the original whatsmeow event
  PRIMARY KEY (chat_jid, id)
);

CREATE INDEX messages_chat_ts_idx    ON messages (chat_jid, timestamp DESC);
CREATE INDEX messages_sender_ts_idx  ON messages (sender_jid, timestamp DESC);
CREATE INDEX messages_ts_idx         ON messages (timestamp DESC);
CREATE INDEX chats_last_msg_idx      ON chats (last_message_at DESC);
CREATE INDEX contacts_pushname_idx   ON contacts (push_name);
CREATE INDEX groups_name_idx         ON groups (name);

CREATE TABLE media (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  message_chat_jid TEXT NOT NULL,
  message_id TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  size INTEGER,
  sha256 TEXT,
  width INTEGER,
  height INTEGER,
  duration_sec INTEGER,
  download_ref TEXT NOT NULL,
  local_path TEXT,
  downloaded_at INTEGER,
  UNIQUE (message_chat_jid, message_id),
  FOREIGN KEY (message_chat_jid, message_id) REFERENCES messages(chat_jid, id)
);
