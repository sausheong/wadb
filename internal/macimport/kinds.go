package macimport

// kindForMessageType maps a Core Data ZMESSAGETYPE integer to wadb's
// messages.kind enum. Mapping is empirical, derived from inspection of
// real WhatsApp Desktop data — values not in the table map to "system",
// which preserves the row without making false claims about content.
//
// The wadb kind enum (see internal/db/migrations/001_initial.up.sql):
//   'text','image','video','audio','voice','document','sticker',
//   'location','contact','system'
func kindForMessageType(t int) string {
	switch t {
	case 0:
		return "text"
	case 1:
		return "image"
	case 2:
		return "video"
	case 3:
		return "audio"
	case 4:
		return "contact"
	case 5:
		return "location"
	case 7:
		return "document"
	case 10:
		return "sticker"
	case 14:
		return "video" // gif — no separate kind in wadb's enum
	case 6, 8, 9, 11, 12, 13, 15, 46, 59, 66:
		return "system"
	default:
		return "system"
	}
}
