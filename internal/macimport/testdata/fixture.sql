-- Two DM chats + one group.
INSERT INTO ZWACHATSESSION (Z_PK, ZSESSIONTYPE, ZARCHIVED, ZUNREADCOUNT, ZLASTMESSAGEDATE, ZCONTACTJID, ZPARTNERNAME, ZGROUPINFO) VALUES
    (1, 0, 0, 0, 759369600.0, '111@s.whatsapp.net', 'Alice', NULL),
    (2, 0, 0, 0, 759369700.0, '222@s.whatsapp.net', 'Bob',   NULL),
    (3, 1, 0, 0, 759369800.0, 'group1@g.us',         'Trip Planning', 1);

INSERT INTO ZWAGROUPINFO (Z_PK, ZCHATSESSION, ZCREATIONDATE, ZOWNERJID) VALUES
    (1, 3, 759369000.0, '111@s.whatsapp.net');

INSERT INTO ZWAGROUPMEMBER (Z_PK, ZCHATSESSION, ZISADMIN, ZMEMBERJID) VALUES
    (1, 3, 1, '111@s.whatsapp.net'),
    (2, 3, 0, '222@s.whatsapp.net'),
    (3, 3, 0, '333@s.whatsapp.net');

-- Messages: text DM, image DM (with media), group msg with reply,
-- system message, unknown-type message, NULL-stanza-id row (skipped).
INSERT INTO ZWAMESSAGE
    (Z_PK, ZCHATSESSION, ZSTANZAID, ZFROMJID, ZTOJID, ZPUSHNAME, ZISFROMME, ZMESSAGEDATE, ZMESSAGETYPE, ZTEXT, ZPARENTMESSAGE, ZMEDIAITEM)
VALUES
    (1, 1, 'MSG-DM-1', '111@s.whatsapp.net', NULL,                  'Alice', 0, 759369500.0, 0,  'hello',     NULL, NULL),
    (2, 1, 'MSG-DM-2', NULL,                 '111@s.whatsapp.net',  NULL,    1, 759369600.0, 0,  'hi back',   NULL, NULL),
    (3, 2, 'MSG-DM-3', '222@s.whatsapp.net', NULL,                  'Bob',   0, 759369700.0, 1,  'check pic', NULL, 1),
    (4, 3, 'MSG-GR-1', '111@s.whatsapp.net', NULL,                  'Alice', 0, 759369800.0, 0,  'kickoff',   NULL, NULL),
    (5, 3, 'MSG-GR-2', '222@s.whatsapp.net', NULL,                  'Bob',   0, 759369900.0, 0,  'sounds good', 4, NULL),
    (6, 3, 'MSG-SYS',  NULL,                 NULL,                  NULL,    0, 759370000.0, 6,  NULL,        NULL, NULL),
    (7, 3, 'MSG-UNKN', '111@s.whatsapp.net', NULL,                  'Alice', 0, 759370100.0, 99, 'mystery',   NULL, NULL),
    (8, 1, NULL,       '111@s.whatsapp.net', NULL,                  'Alice', 0, 759370200.0, 0,  'orphan',    NULL, NULL);

INSERT INTO ZWAMEDIAITEM (Z_PK, ZMESSAGE, ZFILESIZE, ZMOVIEDURATION, ZMEDIALOCALPATH) VALUES
    (1, 3, 102400, 0, 'Message/Media/222@s.whatsapp.net/0/image-001.jpg');

-- Profile push names: authoritative human names. 111 has a real name
-- here that should win over the ZWACHATSESSION.ZPARTNERNAME fallback.
-- 333 only appears as a group participant — no DM row, no profile name.
INSERT INTO ZWAPROFILEPUSHNAME (Z_PK, ZJID, ZPUSHNAME) VALUES
    (1, '111@s.whatsapp.net', 'Alice Anderson'),
    (2, '222@s.whatsapp.net', 'Bob Barker');
