package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/net/proxy"

	"github.com/hadi77ir/nativemessagingproxy/pkg/config"
	_ "github.com/hadi77ir/nativemessagingproxy/pkg/proxy"

	runnable "github.com/hadi77ir/go-runnable"
	"github.com/phayes/freeport"
)

const ToHost = "ToHost"
const ToChrome = "ToChrome"

type receivedMessage struct {
	Body []byte
	Tag  string
}

func RunBridge(c context.Context) error {
	for {
		select {
		case <-c.Done():
			return nil
		default:
		}

		cfg := runnable.ContextConfig(c).(*config.Config)

		// prepare for launching the process
		cmd := exec.Command(cfg.Command)
		inPipe, err := cmd.StdinPipe()
		if err != nil {
			return err
		}
		outPipe, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		errPipe, err := cmd.StderrPipe()
		if err != nil {
			return err
		}

		// prepare a dialer
		var client http.Client
		if cfg.Proxy != "" {
			proxyAddr, err := url.Parse(cfg.Proxy)
			if err != nil {
				return err
			}
			proxyDialer, err := proxy.FromURL(proxyAddr, nil)
			if err != nil {
				return err
			}
			client = http.Client{
				Transport: &http.Transport{
					DialContext: func(_ context.Context, network string, address string) (net.Conn, error) {
						return proxyDialer.Dial(network, address)
					},
				},
			}
		}

		receivedCh := make(chan receivedMessage, 1024)

		// launch an http server
		freePort, err := freeport.GetFreePort()
		if err != nil {
			return err
		}
		httpServer := &http.Server{
			Addr: "127.0.0.1:" + strconv.Itoa(freePort),
		}

		// channels
		errCh := make(chan error, 1)
		closeCh := make(chan struct{})
		defer func() {
			close(closeCh)
			_ = httpServer.Close()
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}()

		// handler
		httpServer.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receiveMessageFromProxy(receivedCh, w, r, errCh, closeCh)
		})

		// listen in background and receive messages tunneled through proxy
		go func() {
			err := httpServer.ListenAndServe()
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()

		// main operation
		serverUrl := fmt.Sprintf("http://127.0.0.1:%d/", freePort)
		// send messages coming from chrome through proxy, to be sent later to native messaging host
		go proxyMessage(client.Post, serverUrl+ToHost, os.Stdin, errCh, closeCh)
		// send messages coming from native messaging host through proxy, to be sent later to chrome
		go proxyMessage(client.Post, serverUrl+ToChrome, outPipe, errCh, closeCh)

		go sendMessageFinal(receivedCh, inPipe, os.Stdout, errCh, closeCh)
		go func() {
			_, err := io.Copy(os.Stderr, errPipe)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()

		select {
		case <-c.Done():
			return nil
		case err := <-errCh:
			return err
		}
	}
}

func sendMessageFinal(receiveChan chan receivedMessage, toHost io.Writer, toChrome io.Writer, errCh chan error, done chan struct{}) {
	for {
		select {
		case received := <-receiveChan:
			var err error
			if strings.EqualFold(received.Tag, ToHost) {
				err = writeMessage(toHost, received.Body)
			} else if strings.EqualFold(received.Tag, ToChrome) {
				err = writeMessage(toChrome, received.Body)
			}
			if err != nil {
				select {
				case <-done:
				case errCh <- err:
				default:
				}
				return
			}
		case <-done:
			return
		}
	}

}

func receiveMessageFromProxy(ch chan receivedMessage, w http.ResponseWriter, r *http.Request, errCh chan error, done chan struct{}) {
	if r.Method == http.MethodPost && r.Header.Get("Content-Type") == "application/json" {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		message := receivedMessage{
			Body: body,
			Tag:  strings.Trim(r.URL.Path, "/"),
		}
		select {
		case ch <- message:
			w.WriteHeader(http.StatusOK)
			return
		case <-done:
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusBadRequest)
}

type PostFunc func(url string, contentType string, body io.Reader) (*http.Response, error)

func proxyMessage(dst PostFunc, target string, src io.Reader, errCh chan error, done <-chan struct{}) {
	lenBuf := make([]byte, 4)
	sendError := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}
	for {
		select {
		case <-done:
			return
		default:
		}
		_, err := io.ReadAtLeast(src, lenBuf, 4)
		if err != nil {
			sendError(err)
			return
		}
		buf := make([]byte, 4)
		msgLen := int(binary.BigEndian.Uint32(buf))
		if msgLen > 8*1024*1024 {
			sendError(err)
			return
		}
		msgBuf := make([]byte, msgLen)
		_, err = io.ReadAtLeast(src, msgBuf, msgLen)
		if err != nil {
			sendError(err)
			return
		}

		// done reading, now send over http to receiver
		reader := bytes.NewReader(msgBuf)
		resp, err := dst(target, "application/json", reader)
		if err != nil {
			sendError(err)
			return
		}
		_ = resp.Body.Close()
	}
}

func writeMessage(writer io.Writer, body []byte) error {
	lenBuf := make([]byte, 4)
	binary.NativeEndian.PutUint32(lenBuf, uint32(len(body)))

	_, err := writer.Write(lenBuf)
	if err != nil {
		return err
	}
	_, err = writer.Write(body)
	if err != nil {
		return err
	}
	return nil
}
