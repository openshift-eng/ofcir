package commands

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	ofcirv1 "github.com/openshift/ofcir/api/v1"
	ofcirclientv1 "github.com/openshift/ofcir/pkg/server/clientset/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type releaseCmd struct {
	context   *gin.Context
	clientset *ofcirclientv1.OfcirV1Client
	namespace string
	cirName   string
}

func NewReleaseCmd(c *gin.Context, clientset *ofcirclientv1.OfcirV1Client, ns string, cirName string) command {
	return &releaseCmd{
		context:   c,
		clientset: clientset,
		namespace: ns,
		cirName:   cirName,
	}
}

func (c *releaseCmd) Run() error {

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

	switch r.Status.State {
	case ofcirv1.StateInUse:
		r.Spec.State = ofcirv1.StateAvailable
		_, err := c.clientset.CIResources(r.Namespace).Update(context.Background(), r, v1.UpdateOptions{})
		if err != nil {
			return err
		}

		c.context.String(http.StatusOK, r.Name)

	default:
		c.context.JSON(http.StatusBadRequest, gin.H{
			"msg": fmt.Sprintf("%s state must be `%s`, but it is `%s`", c.cirName, ofcirv1.StateInUse, r.Status.State),
		})
	}

	return nil
}
