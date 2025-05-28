package types

type Swap struct {
	Amount string `json:"amount"`
	Date   string `json:"date"`
	Hour   string `json:"hour"`
	Id     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
}
