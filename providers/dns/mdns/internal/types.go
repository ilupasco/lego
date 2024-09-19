package internal

const (
	DefaultBaseURL  = "https://mdns.nic.md/api/"
	HeaderAuthToken = "Authorization"
	HeaderAuthEmail = "Email"
)

type Response struct {
	Status   string   `json:"status,omitempty"`
	Messages string   `json:"messages,omitempty"`
	Results  []Record `json:"results,omitempty"`
}

type Record struct {
	ID      int    `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Content string `json:"content,omitempty"`
}

type APIError struct {
	Message string `json:"message"`
}

func (a APIError) Error() string {
	return a.Message
}
