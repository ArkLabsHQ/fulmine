package handlers

import "github.com/gin-gonic/gin"

func getBalance() string {
	return "1930547"
}

func getAddress() string {
	return "ark18746676365652bcdabdbacdabcd63546354634"
}

func getTransactions() [][]string {
	var transactions [][]string
	transactions = append(transactions, []string{"abcd", "waiting", "08/08/2024", "21:42", "+212110"})
	transactions = append(transactions, []string{"1234", "sent", "08/08/2024", "21:42", "-645543"})
	transactions = append(transactions, []string{"ab12", "sent", "07/08/2024", "21:42", "-645543"})
	transactions = append(transactions, []string{"cd21", "received", "06/08/2024", "21:42", "+56632"})
	transactions = append(transactions, []string{"ffee", "received", "05/08/2024", "21:42", "+655255"})
	transactions = append(transactions, []string{"445d", "swap", "04/08/2024", "21:42", "+42334"})
	return transactions
}

func redirect(path string, c *gin.Context) {
	c.Header("HX-Redirect", path)
	c.Status(303)
}
