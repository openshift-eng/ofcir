package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/openshift/ofcir/pkg/server/commands"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	ofcirclientv1 "github.com/openshift/ofcir/pkg/server/clientset/v1"
)

type OfcirAPI struct {
	sync.Mutex
	config    *rest.Config
	clientset *ofcirclientv1.OfcirV1Client
	corev1    corev1.CoreV1Interface
	router    *gin.Engine

	port      string
	namespace string
}

func NewOfcirAPI(port string, namespace string) *OfcirAPI {
	return &OfcirAPI{
		port:      port,
		namespace: namespace,
	}
}

func (o *OfcirAPI) Init(kubeconfig string) error {

	var err error
	var config *rest.Config

	// Use this option when running outside the cluster
	if kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		return err
	}
	o.config = config

	ofcirv1.AddToScheme(scheme.Scheme)

	// create the clientset
	clientset, err := ofcirclientv1.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	o.clientset = clientset

	kubeclient, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	o.corev1 = kubeclient.CoreV1()

	// Setup the server
	r := gin.Default()
	r.Group("/v1").Use(o.AuthRequired()).
		GET("/ofcir/:cirName", o.handleGetCirStatus).
		POST("/ofcir", o.handleAcquireCir).
		DELETE("/ofcir/:cirName", o.handleReleaseCir)

	o.router = r
	return nil
}

func (o *OfcirAPI) AuthRequired() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		tokens, err := o.corev1.Secrets("ofcir-system").Get(context.Background(), "ofcir-tokens", metav1.GetOptions{})
		tokenheader := ctx.Request.Header["X-Ofcirtoken"]
		if (err != nil) || (len(tokenheader) == 0) {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		if string(tokens.Data[tokenheader[0]]) == "" {
			ctx.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		ctx.Set("validpools", strings.TrimSpace(string(tokens.Data[tokenheader[0]])))
	}
}

func (o *OfcirAPI) Run() {
	o.router.Run(fmt.Sprintf(":%s", o.port))
}

func (o *OfcirAPI) handleGetCirStatus(c *gin.Context) {
	cirName := c.Param("cirName")
	cmd := commands.NewStatusCmd(c, o.clientset, o.namespace, cirName)
	if err := cmd.Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"msg": err.Error(),
		})
	}
}

func (o *OfcirAPI) handleAcquireCir(c *gin.Context) {
	o.Lock()
	defer o.Unlock()

	resourceType := c.DefaultQuery("type", string(ofcirv1.TypeCIHost))
	cmd := commands.NewAcquireCmd(c, o.clientset, o.namespace, resourceType)
	if err := cmd.Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"msg": err.Error(),
		})
	}
}

func (o *OfcirAPI) handleReleaseCir(c *gin.Context) {
	o.Lock()
	defer o.Unlock()

	cirName := c.Param("cirName")
	cmd := commands.NewReleaseCmd(c, o.clientset, o.namespace, cirName)
	if err := cmd.Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"msg": err.Error(),
		})
	}
}
