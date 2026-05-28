CREATE VIRTUAL TABLE fts_messages USING fts5(
  text,
  content='messages',
  content_rowid='rowid',
  tokenize='unicode61'
);

CREATE TRIGGER messages_ai AFTER INSERT ON messages BEGIN
  INSERT INTO fts_messages(rowid, text) VALUES (new.rowid, COALESCE(new.text, ''));
END;

CREATE TRIGGER messages_ad AFTER DELETE ON messages BEGIN
  INSERT INTO fts_messages(fts_messages, rowid, text) VALUES('delete', old.rowid, COALESCE(old.text, ''));
END;

CREATE TRIGGER messages_au AFTER UPDATE ON messages BEGIN
  INSERT INTO fts_messages(fts_messages, rowid, text) VALUES('delete', old.rowid, COALESCE(old.text, ''));
  INSERT INTO fts_messages(rowid, text) VALUES (new.rowid, COALESCE(new.text, ''));
END;
