package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"regexp"
	"sync"
	"time"

	"github.com/wowoniu/go_test/proxy/protocol"
)

type Tunnel struct {
	ClientConn    net.Conn
	LocalResponse chan *localResponse
}

type Request struct {
	CreateTime time.Time
	Conn       net.Conn
}

type localResponse struct {
	RequestID []byte
	Response  []byte
	IsOver    bool
}

var (
	proxyTunnel                   *Tunnel
	serverAddr                    string
	proxyPort                     string
	proxyHost                     string
	proxySession                  map[string]*Request
	buffSize                      int = 10240
	tmpData                           = make([]byte, 0)
	proxySessionLock              sync.Mutex
	proxyRequestQueenChan         = make(chan []byte, 200)
	proxyRequestQueenControllChan = make(chan bool, 5)
)

func main() {
	flag.StringVar(&serverAddr, "serverAddr", "localhost:9000", "代理服务器地址")
	flag.StringVar(&proxyPort, "proxyPort", "80", "代理端口")
	flag.StringVar(&proxyHost, "proxyHost", "localhost", "代理本地域名")
	flag.Parse()
	log.Println("代理本地域名:", proxyHost+":"+proxyPort)
	proxySession = make(map[string]*Request)
	defer func() {
		for _, proxyRequest := range proxySession {
			proxyRequest.Conn.Close()
		}
	}()

	connectToServer()
}

func connectToServer() {
	//tcpAddr, _ := net.ResolveTCPAddr("tcp4", serverAddr)
	log.Println("connect to ", serverAddr, "...")
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		log.Fatal("connected error :", err)
	}
	defer conn.Close()
	log.Println("server connect success")
	//与服务器建立连接通道
	proxyTunnel = &Tunnel{
		ClientConn:    conn,
		LocalResponse: make(chan *localResponse, 1000),
	}

	go handleLocalResponse()
	go handleRequest()
	//读取服务器代理请求消息
	tmpBuffer := make([]byte, 0)
	for {
		buff := make([]byte, 1024)
		data := make([]byte, 0)
		if n, err := conn.Read(buff); err != nil {
			log.Fatal("READ FROM SERVER ERROR:", err)
			return
		} else {
			tmpBuffer, data = protocol.Unpack(append(tmpBuffer, buff[:n]...))
			if len(data) > 0 {
				proxyRequestQueenChan <- data
			}

		}
	}

}

func handleRequest() {
	for data := range proxyRequestQueenChan {
		//proxyRequestQueenControllChan <- true
		go getRequestFromLocal(data)
	}
}

func getRequestFromLocal(data []byte) {
	defer func() {
		//流控
		// if len(proxyRequestQueenControllChan) > 0 {
		// 	<-proxyRequestQueenControllChan
		// }
	}()
	requestID := data[:16]
	requestData := data[16:]
	log.Printf("接收到代理请求:%x", requestID)
	//加入本地代理请求队列

	log.Printf("接收到代理请求:%s", requestData)
	conn, err := net.Dial("tcp", "localhost:"+proxyPort)
	if err != nil {
		log.Println("connected error :", err)
		return
	}
	defer conn.Close()
	log.Println("local connect success:", fmt.Sprintf("%x", requestID))
	proxySession[fmt.Sprintf("%x", requestID)] = &Request{
		CreateTime: time.Now(),
		Conn:       conn,
	}
	//读取本地连接的响应
	str := string(requestData)
	//host替换
	reg := regexp.MustCompile(`Host: ([\s\S]*?)[:\d+]?\r\n`)
	str = reg.ReplaceAllString(str, "Host: "+proxyHost+":"+proxyPort+"\r\n")
	// 本地连接 写入请求
	conn.Write([]byte(str))
	//读取本地连接的响应

	conn.SetReadDeadline(time.Now().Add(time.Second * 5))
	n := 0
	for {
		buff := make([]byte, buffSize)
		if n, err = conn.Read(buff); err != nil {
			//本地连接已关闭
			log.Println("本地响应读取结束:", err)
			response := protocol.Packet(append(requestID, []byte("CLOSE")...))
			pushLocalResponse(requestID, response, true)
			closeLocalConn(requestID)
			return
		} else {
			response := protocol.Packet(append(requestID, buff[:n]...))
			// requestIsOver := n < buffSize
			//log.Println("本地响应:", n)
			pushLocalResponse(requestID, response, false)
			// if requestIsOver {
			// 	log.Println("本地响应结束:", n)
			// 	log.Printf("%s", buff[:n])
			// 	//发一个代理请求结束的通知包
			// 	response := protocol.Packet(append(requestID, []byte("CLOSE")...))
			// 	pushLocalResponse(requestID, response, true)
			// 	closeLocalConn(requestID)
			// 	//return
			// }

		}
	}
}

func handleLocalResponse() {
	for response := range proxyTunnel.LocalResponse {
		proxyTunnel.ClientConn.Write(response.Response)
		if response.IsOver {
			closeLocalConn(response.RequestID)
		}
	}
}

func closeLocalConn(requestID []byte) {
	requestIDStr := fmt.Sprintf("%x", requestID)
	proxySessionLock.Lock()
	if proxyRequest, ok := proxySession[requestIDStr]; ok {
		proxyRequest.Conn.Close()
		delete(proxySession, requestIDStr)
		//log.Printf("关闭本地连接：%s", requestIDStr)
	}
	proxySessionLock.Unlock()
}

func pushLocalResponse(requestID []byte, data []byte, isOver bool) {
	proxyTunnel.LocalResponse <- &localResponse{
		RequestID: requestID,
		Response:  data,
		IsOver:    isOver,
	}
}
