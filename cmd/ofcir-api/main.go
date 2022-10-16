package main

import (
	"flag"

	"github.com/openshift/ofcir/pkg/server"
)

func main() {
	var kubeconfig, port, namespace string
	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&port, "port", "8087", "server port")
	flag.StringVar(&namespace, "namespace", "ofcir-system", "Namespace to look for CIPool and CIR resources")
	flag.Parse()

	srv := server.NewOfcirAPI(port, namespace)
	if err := srv.Init(kubeconfig); err != nil {
		panic(err.Error())
	}

	srv.Run()
}
