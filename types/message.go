package types

type Message struct {
	ChatID int64
	URL    string
	// Presentation fields must not change the persisted deduplication key.
	Title    string `json:"-"`
	Text     string `json:"-"`
	ImageURL string `json:"-"`
}
