package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func GetExplorerUrl(c *gin.Context) string {
	url := "http://localhost:5000" // default for regtest
	if storeSvc, err := openStore(); err == nil {
		if data, err := storeSvc.GetData(c); err == nil {
			logrus.Infof("network %v", data.Network.Name)
			switch data.Network.Name {
			case "liquid":
				url = "http://liquid.network"
			case "bitcoin":
				url = "http://mempool.space"
			case "signet":
				url = "http://mutinynet.com"
			case "regtest":
				url = "http://localhost:5000"
			}
		}
	}
	return url
}
