package firefox

import "encoding/json"

type actorMessage struct {
	To   string `json:"to,omitempty"`
	From string `json:"from,omitempty"`
	Type string `json:"type,omitempty"`

	Tabs    []*actorTab       `json:"tabs,omitempty"`
	Frame   *actorFrame       `json:"frame,omitempty"`
	Title   string            `json:"title,omitempty"`
	URL     string            `json:"url,omitempty"`
	State   string            `json:"state,omitempty"`
	Favicon actorFaviconBytes `json:"favicon,omitempty"`
}

type actorTab struct {
	Actor    string `json:"actor,omitempty"`
	Selected bool   `json:"selected,omitempty"`
	Title    string `json:"title,omitempty"`
	URL      string `json:"url,omitempty"`
}

type actorFrame struct {
	Actor string `json:"actor,omitempty"`
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
}

type actorFaviconBytes struct {
	set   bool
	bytes []uint8
}

func (a *actorFaviconBytes) UnmarshalJSON(data []byte) error {
	a.set = true
	return json.Unmarshal(data, &a.bytes)
}
