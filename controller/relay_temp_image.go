package controller

import (
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func ServeRelayTempImage(c *gin.Context) {
	service.ServeRelayTempImage(c)
}
