package commands

import (
	"context"
	"fmt"
	"github.com/openshift/ofcir/pkg/utils"
	"net/http"

	"github.com/gin-gonic/gin"
	ofcirclientv1 "github.com/openshift/ofcir/pkg/server/clientset/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type statusCmd struct {
	context   *gin.Context
	clientset *ofcirclientv1.OfcirV1Client
	namespace string
	cirName   string
}

func NewStatusCmd(c *gin.Context, clientset *ofcirclientv1.OfcirV1Client, ns string, cirName string) command {
	return &statusCmd{
		context:   c,
		clientset: clientset,
		namespace: ns,
		cirName:   cirName,
	}
}

func (c *statusCmd) Run() error {

	r, err := c.clientset.CIResources(c.namespace).Get(context.Background(), c.cirName, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			c.context.JSON(http.StatusBadRequest, gin.H{
				"msg": fmt.Sprintf("%s does not exist in namespace %s", c.cirName, c.namespace),
			})
			return nil
		}
		return err
	}

	if !utils.CanUsePool(c.context, r.Spec.PoolRef.Name) {
		c.context.AbortWithStatus(http.StatusUnauthorized)
		return nil
	}

	pool, err := c.clientset.CIPools(c.namespace).Get(context.Background(), r.Spec.PoolRef.Name, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			c.context.JSON(http.StatusBadRequest, gin.H{
				"msg": fmt.Sprintf("Cannot find cipool %s for %s in namespace %s", r.Spec.PoolRef.Name, c.cirName, c.namespace),
			})
			return nil
		}
		return err
	}

	c.context.JSON(http.StatusOK, gin.H{
		"name":         r.Name,
		"pool":         pool.Name,
		"provider":     pool.Spec.Provider,
		"providerInfo": r.Status.ProviderInfo,
		"type":         r.Spec.Type,
		"ip":           r.Status.Address,
		"extra":        r.Status.Extra,
		"status":       r.Status.State,
	})

	return nil
}
