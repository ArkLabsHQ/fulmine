package handlers

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ArkLabsHQ/ark-wallet/internal/interface/web/types"
)

func getAddress() string {
	return "ark18746676365652bcdabdbacdabcd63546354634"
}

func getBalance() string {
	return "1930547"
}

func getNodeBalance() string {
	return "50640"
}

func getNewMnemonic() []string {
	mnemonic := "ski this panic exit erode peasant nose swim spell sleep unique bag"
	return strings.Fields(mnemonic)
}

func getSettings() types.Settings {
	return types.Settings{
		ApiRoot:     "https://fulmine.io/api/D9D90N192031",
		Currency:    "usd",
		FullNode:    "http://arklabs.to/node/213908123",
		LnUrl:       "lndconnect://192.168.1.4:10009?cert=MIICiDCCAi-gAwIBAgIQdo5v0QBXHnji4hRaeeMjNDAKBggqhkjOPQQDAjBHMR8wHQYDVQQKExZsbmQgYXV0b2dlbmVyYXRlZCBjZXJ0MSQwIgYDVQQDExtKdXN0dXNzLU1hY0Jvb2stUHJvLTMubG9jYWwwHhcNMTgwODIzMDU1ODEwWhcNMTkxMDE4MDU1ODEwWjBHMR8wHQYDVQQKExZsbmQgYXV0b2dlbmVyYXRlZCBjZXJ0MSQwIgYDVQQDExtKdXN0dXNzLU1hY0Jvb2stUHJvLTMubG9jYWwwWTATBgcqhkjOPQIBBggqhkiOPQMBBwNCAASFhRm-w_T10PoKtg4lm9hBNJjJD473fkzHwPUFwy91vTrQSf7543j2JrgFo8mbTV0VtpgqkfK1IMVKMLrF21xio4H8MIH5MA4GA1UdDwEB_wQEAwICpDAPBgNVHRMBAf8EBTADAQH_MIHVBgNVHREEgc0wgcqCG0p1c3R1c3MtTWFjQm9vay1Qcm8tMy5sb2NhbIIJbG9jYWxob3N0ggR1bml4ggp1bml4cGFja2V0hwR_AAABhxAAAAAAAAAAAAAAAAAAAAABhxD-gAAAAAAAAAAAAAAAAAABhxD-gAAAAAAAAAwlc9Zck7bDhwTAqAEEhxD-gAAAAAAAABiNp__-GxXGhxD-gAAAAAAAAKWJ5tliDORjhwQKDwAChxD-gAAAAAAAAG6Wz__-3atFhxD92tDQyv4TAQAAAAAAABAAMAoGCCqGSM49BAMCA0cAMEQCIA9O9xtazmdxCKj0MfbFHVBq5I7JMnOFPpwRPJXQfrYaAiBd5NyJQCwlSx5ECnPOH5sRpv26T8aUcXbmynx9CoDufA&macaroon=AgEDbG5kArsBAwoQ3_I9f6kgSE6aUPd85lWpOBIBMBoWCgdhZGRyZXNzEgRyZWFkEgV3cml0ZRoTCgRpbmZvEgRyZWFkEgV32ml0ZRoXCghpbnZvaWNlcxIEcmVhZBIFd3JpdGUaFgoHbWVzc2FnZRIEcmVhZBIFd3JpdGUaFwoIb2ZmY2hhaW4SBHJlYWQSBXdyaXRlGhYKB29uY2hhaW4SBHJlYWQSBXdyaXRlGhQKBXBlZXJzEgRyZWFkEgV3cml0ZQAABiAiUTBv3Eh6iDbdjmXCfNxp4HBEcOYNzXhrm-ncLHf5jA",
		EventServer: "http://arklabs.to/node/jupiter29",
		Unit:        "sat",
	}
}

func getTransactions() [][]string {
	var transactions [][]string
	transactions = append(transactions, []string{"cd21", "send", "pending", "10/08/2024", "21:42", "+56632"})
	transactions = append(transactions, []string{"abcd", "send", "waiting", "09/08/2024", "21:42", "+212110"})
	transactions = append(transactions, []string{"1234", "send", "success", "08/08/2024", "21:42", "-645543"})
	transactions = append(transactions, []string{"ab12", "send", "success", "07/08/2024", "21:42", "-645543"})
	transactions = append(transactions, []string{"f3f3", "recv", "success", "06/08/2024", "21:42", "+56632"})
	transactions = append(transactions, []string{"ffee", "recv", "failure", "05/08/2024", "21:42", "+655255"})
	transactions = append(transactions, []string{"445d", "swap", "success", "04/08/2024", "21:42", "+42334"})
	return transactions
}

func redirect(path string, c *gin.Context) {
	c.Header("HX-Redirect", path)
	c.Status(303)
}
