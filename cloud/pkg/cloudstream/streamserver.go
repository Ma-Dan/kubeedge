/*
Copyright 2020 The KubeEdge Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloudstream

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"

	"github.com/emicklei/go-restful"
	"k8s.io/klog"

	"github.com/kubeedge/kubeedge/cloud/pkg/cloudstream/config"
	"github.com/kubeedge/kubeedge/pkg/stream/flushwriter"
)

type StreamServer struct {
	// nextMessageID indicates the next message id
	// it starts from 0 , when receive a new apiserver connection and then add 1
	nextMessageID uint64
	container     *restful.Container
	tunnel        *TunnelServer
}

func newStreamServer(t *TunnelServer) *StreamServer {
	return &StreamServer{
		container: restful.NewContainer(),
		tunnel:    t,
	}
}

func (s *StreamServer) installDebugHandler() {
	ws := new(restful.WebService)
	ws.Path("/containerLogs")
	ws.Route(ws.GET("/{podNamespace}/{podID}/{containerName}").
		To(s.getContainerLogs))
	s.container.Add(ws)

	ws = new(restful.WebService)
	ws.Path("/exec")

	ws.Route(ws.GET("/{podNamespace}/{podID}/{containerName}").
		To(s.getExec))
	ws.Route(ws.POST("/{podNamespace}/{podID}/{containerName}").
		To(s.getExec))
	ws.Route(ws.GET("/{podNamespace}/{podID}/{uid}/{containerName}").
		To(s.getExec))
	ws.Route(ws.POST("/{podNamespace}/{podID}/{uid}/{containerName}").
		To(s.getExec))
	s.container.Add(ws)

	ws = new(restful.WebService)
	ws.Path("/stats")
	ws.Route(ws.GET("/summary").
		To(s.getMetricsSummary))
	s.container.Add(ws)
}

func (s *StreamServer) getExec(r *restful.Request, w *restful.Response) {
	// TODO @kadisi
}

func (s *StreamServer) getContainerLogs(r *restful.Request, w *restful.Response) {
	var err error

	defer func() {
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			klog.Errorf(err.Error())
		}
	}()

	sessionKey := strings.Split(r.Request.Host, ":")[0]
	session, ok := s.tunnel.getSession(sessionKey)
	if !ok {
		err = fmt.Errorf("Can not find %v session ", sessionKey)
		return
	}

	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	if _, ok := w.ResponseWriter.(http.Flusher); !ok {
		err = fmt.Errorf("Unable to convert %v into http.Flusher, cannot show logs", reflect.TypeOf(w))
		return
	}
	fw := flushwriter.Wrap(w.ResponseWriter)

	logConnection, err := session.AddAPIServerConnection(s, &ContainerLogsConnection{
		r:            r,
		flush:        fw,
		session:      session,
		ctx:          r.Request.Context(),
		edgePeerStop: make(chan struct{}),
	})
	if err != nil {
		klog.Errorf("Add apiserver connection into %s error %v", session.String(), err)
		return
	}

	defer func() {
		session.DeleteAPIServerConnection(logConnection)
		klog.Infof("Delete %s from %s", logConnection.String(), session.String())
	}()

	if err := logConnection.Serve(); err != nil {
		err = fmt.Errorf("apiconnection Serve %s in %s error %v",
			logConnection.String(), session.String(), err)
		return
	}
}

func (s *StreamServer) getMetricsSummary(r *restful.Request, w *restful.Response) {
	var err error

	defer func() {
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			klog.Errorf(err.Error())
		}
	}()

	sessionKey := strings.Split(r.Request.Host, ":")[0]
	session, ok := s.tunnel.getSession(sessionKey)
	if !ok {
		err = fmt.Errorf("Can not find %v session ", sessionKey)
		return
	}

	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	if _, ok := w.ResponseWriter.(http.Flusher); !ok {
		err = fmt.Errorf("Unable to convert %v into http.Flusher, cannot show logs", reflect.TypeOf(w))
		return
	}
	fw := flushwriter.Wrap(w.ResponseWriter)

	metricsConnection, err := session.AddAPIServerConnection(s, &EdgeMetricsConnection{
		r:            r,
		flush:        fw,
		session:      session,
		ctx:          r.Request.Context(),
		edgePeerStop: make(chan struct{}),
	})
	if err != nil {
		klog.Errorf("Add apiserver connection into %s error %v", session.String(), err)
		return
	}

	defer func() {
		session.DeleteAPIServerConnection(metricsConnection)
		klog.Infof("Delete %s from %s", metricsConnection.String(), session.String())
	}()

	if err := metricsConnection.Serve(); err != nil {
		err = fmt.Errorf("apiconnection Serve %s in %s error %v",
			metricsConnection.String(), session.String(), err)
		return
	}
}

func (s *StreamServer) Start() {
	s.installDebugHandler()

	pool := x509.NewCertPool()
	data, err := ioutil.ReadFile(config.Config.TLSStreamCAFile)
	if err != nil {
		klog.Fatalf("Read tls stream ca file error %v", err)
		return
	}
	pool.AppendCertsFromPEM(data)

	tunnelServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Config.StreamPort),
		Handler: s.container,
		TLSConfig: &tls.Config{
			ClientCAs: pool,
			// Populate PeerCertificates in requests, but don't reject connections without verified certificates
			ClientAuth: tls.RequestClientCert,
		},
	}
	klog.Infof("Prepare to start stream server ...")
	err = tunnelServer.ListenAndServeTLS(config.Config.TLSStreamCertFile, config.Config.TLSStreamPrivateKeyFile)
	if err != nil {
		klog.Fatalf("Start stream server error %v\n", err)
		return
	}
}
