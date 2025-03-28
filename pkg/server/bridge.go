package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/hadi77ir/go-logging"
	"github.com/hadi77ir/nativemessagingproxy/pkg/log"
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

// MaxMessageSize is 1MB, per Chrome documentation
const MaxMessageSize = 1 * 1024 * 1024

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
		logger := log.Global()

		receivedCh := make(chan receivedMessage, 1024)

		stdinReader := bufio.NewReaderSize(bufio.NewReader(os.Stdin), MaxMessageSize+4)

		logger.Log(logging.TraceLevel, "prepare for launching the process")

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

		logger.Log(logging.TraceLevel, "preparing dialer, proxy = ", cfg.Proxy)
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

		logger.Log(logging.DebugLevel, "prepare http server")
		freePort, err := freeport.GetFreePort()
		if err != nil {
			return err
		}
		httpServer := &http.Server{
			Addr: "127.0.0.1:" + strconv.Itoa(freePort),
		}
		logger.Log(logging.TraceLevel, "prepared http server. http server will listen at "+httpServer.Addr)

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
			receiveMessageFromProxy(receivedCh, w, r, logger, errCh, closeCh)
		})

		go func() {
			err := httpServer.ListenAndServe()
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()
		go func() {
			err := cmd.Start()
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()

		serverUrl := fmt.Sprintf("http://%s/", httpServer.Addr)
		logger.Log(logging.InfoLevel, "started http server at "+serverUrl)

		// main operation
		// send messages coming from chrome through proxy, to be sent later to native messaging host
		go proxyMessage(client.Post, serverUrl+ToHost, stdinReader, logger, errCh, closeCh)
		// send messages coming from native messaging host through proxy, to be sent later to chrome
		go proxyMessage(client.Post, serverUrl+ToChrome, outPipe, logger, errCh, closeCh)

		// send any message from queue channel to final destination
		go sendMessageFinal(receivedCh, inPipe, os.Stdout, logger, errCh, closeCh)

		// pipe errors from process stderr to logger
		go pipeErrors(logger, errPipe, errCh, closeCh)

		logger.Log(logging.InfoLevel, "proxy is running...")

		select {
		case <-c.Done():
			return nil
		case err := <-errCh:
			return err
		}
	}
}

func pipeErrors(logger logging.Logger, pipe io.ReadCloser, errCh chan error, done chan struct{}) {
	scanner := bufio.NewScanner(pipe)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		logger.Log(logging.InfoLevel, fmt.Sprint("received on stderr: ", scanner.Text()))
		select {
		case <-done:
			return
		}
	}
}

func sendMessageFinal(receiveChan chan receivedMessage, toHost io.Writer, toChrome io.Writer, logger logging.Logger, errCh chan error, done chan struct{}) {
	for {
		select {
		case received := <-receiveChan:
			var err error
			logger.Log(logging.DebugLevel, "processing proxied message tag = ", received.Tag, " len = ", len(received.Body))
			if strings.EqualFold(received.Tag, ToHost) {
				logger.Log(logging.DebugLevel, "sending message to host, len = ", len(received.Body))
				err = writeMessage(toHost, received.Body)
			} else if strings.EqualFold(received.Tag, ToChrome) {
				logger.Log(logging.DebugLevel, "sending message to chrome, len = ", len(received.Body))
				err = writeMessage(toChrome, received.Body)
			}
			if err != nil {
				logger.Log(logging.ErrorLevel, "failed to send message to final destination, err = ", err)
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

func receiveMessageFromProxy(ch chan receivedMessage, w http.ResponseWriter, r *http.Request, logger logging.Logger, errCh chan error, done chan struct{}) {
	if r.Method == http.MethodPost && r.Header.Get("Content-Type") == "application/json" {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Log(logging.ErrorLevel, "failed to read message body, err = ", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		message := receivedMessage{
			Body: body,
			Tag:  strings.Trim(r.URL.Path, "/"),
		}
		logger.Log(logging.DebugLevel, "received message in the server, putting into queue, len = ", len(message.Body), " tag = ", message.Tag)
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

func proxyMessage(dst PostFunc, target string, src io.Reader, logger logging.Logger, errCh chan error, done <-chan struct{}) {
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
			logger.Log(logging.ErrorLevel, "got error reading message len, err = ", err)
			sendError(err)
			return
		}

		msgLen := int(binary.NativeEndian.Uint32(lenBuf))
		logger.Log(logging.TraceLevel, "received msgLen, len = ", msgLen)
		if msgLen > MaxMessageSize {
			err = fmt.Errorf("message too large (%d > %d)", msgLen, MaxMessageSize)
			logger.Log(logging.WarnLevel, err)
			sendError(err)
			return
		}

		msgBuf := make([]byte, msgLen)
		n, err := io.ReadAtLeast(src, msgBuf, msgLen)
		if err != nil {
			logger.Log(logging.ErrorLevel, "got error reading message body, err = ", err)
			sendError(err)
			return
		}
		logger.Log(logging.TraceLevel, "read message, len = ", n)

		// done reading, now send over http to receiver
		reader := bytes.NewReader(msgBuf)
		resp, err := dst(target, "application/json", reader)
		if err != nil {
			logger.Log(logging.ErrorLevel, "failed to send message over proxy, err = ", err)
			sendError(err)
			return
		}
		logger.Log(logging.DebugLevel, "sent message over proxy")
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
