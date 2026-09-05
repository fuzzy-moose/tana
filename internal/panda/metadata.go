package panda

// Metadata retains the upstream fields without applying Tana's title or tag
// conventions. When Error is nonempty, only ID and Error are meaningful.
type Metadata struct {
	ID            int64     `json:"gid"`
	Token         string    `json:"token"`
	Error         string    `json:"error"`
	Title         string    `json:"title"`
	TitleJapanese string    `json:"title_jpn"`
	Category      string    `json:"category"`
	ThumbnailURL  string    `json:"thumb"`
	Uploader      string    `json:"uploader"`
	Posted        int64     `json:"posted,string"` // Unix seconds, UTC.
	FileCount     int64     `json:"filecount,string"`
	FileSize      int64     `json:"filesize"` // Bytes.
	Expunged      bool      `json:"expunged"`
	Rating        float64   `json:"rating,string"`
	TorrentCount  int64     `json:"torrentcount,string"`
	Torrents      []Torrent `json:"torrents"`
	Tags          []string  `json:"tags"`
	ParentID      int64     `json:"parent_gid,string"`
	ParentToken   string    `json:"parent_key"`
	CurrentID     int64     `json:"current_gid,string"`
	CurrentToken  string    `json:"current_key"`
	FirstID       int64     `json:"first_gid,string"`
	FirstToken    string    `json:"first_key"`
}

type Torrent struct {
	Hash        string `json:"hash"`
	Added       int64  `json:"added,string"` // Unix seconds, UTC.
	Name        string `json:"name"`
	TorrentSize int64  `json:"tsize,string"` // Bytes.
	FileSize    int64  `json:"fsize,string"` // Bytes.
}
