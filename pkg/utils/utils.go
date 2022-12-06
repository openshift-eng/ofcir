package utils

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

func contains(elems []string, v string) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}

func CanUsePool(context *gin.Context, pool string) bool {
	v, _ := context.Get("validpools")
	if v == nil {
		return false
	}
	if v == "*" {
		return true
	}
	validpools := strings.Split(fmt.Sprint(v), ",")
	return contains(validpools, pool)
}
