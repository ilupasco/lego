package internal

type AddRecordResponse struct {
	Results []Record `json:"results"`
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
