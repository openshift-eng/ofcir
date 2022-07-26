package main

import (
	"flag"

	"github.com/openshift/ofcir/pkg/server"
)

func main() {
	var kubeconfig, port string
	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&port, "port", "8085", "server port")
	flag.Parse()

	srv := server.NewOfcirAPI(port)
	if err := srv.Init(kubeconfig); err != nil {
		panic(err.Error())
	}

	srv.Run()
}
