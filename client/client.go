package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"regexp"
	"time"
)

type Tunnel struct {
	ClientConn net.Conn
}

var (
	proxyTunnel  *Tunnel
	serverAddr   string
	proxyPort    string
	proxySession map[string]net.Conn
	buffSize     int = 102400
)

func main() {
	flag.StringVar(&serverAddr, "serverAddr", "localhost:9000", "代理服务器地址")
	flag.StringVar(&proxyPort, "proxyPort", "80", "代理端口")
	proxySession = make(map[string]net.Conn)
	defer func() {
		for _, conn := range proxySession {
			conn.Close()
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
		ClientConn: conn,
	}

	//读取服务器代理请求消息
	for {
		buff := make([]byte, 1024)
		if n, err := conn.Read(buff); err != nil {
			log.Fatal("READ FROM SERVER ERROR:", err)
			return
		} else {
			go handleRequest(buff[:n])
		}
	}

}

func handleRequest(data []byte) {
	requestID := data[:16]
	requestData := data[16:]
	log.Printf("接收到代理请求:%x", requestID)
	// log.Printf("接收到代理请求:%s", requestData)
	conn, err := net.Dial("tcp", "localhost:"+proxyPort)
	if err != nil {
		log.Println("connected error :", err)
		return
	}
	defer conn.Close()
	log.Println("local connect success:", fmt.Sprintf("%x", requestID))
	proxySession[string(requestID)] = conn
	//读取本地连接的响应
	str := string(requestData)
	//host替换
	reg := regexp.MustCompile(`Host: ([\s\S]*?)[:\d+]?\r\n`)
	str = reg.ReplaceAllString(str, "Host: local.jjs.ie:80\r\n")
	// 本地连接 写入请求
	conn.Write([]byte(str))
	//读取本地连接的响应

	for {
		buff := make([]byte, buffSize)
		conn.SetReadDeadline(time.Now().Add(time.Second * 5))
		if n, err := conn.Read(buff); err != nil {
			//本地连接已关闭
			log.Println("READ FROM LOCAL ERROR:", err)
			go handleLocalResponse(requestID, []byte(""))
			return
		} else {
			log.Println("请求长度:", n)
			response := make([]byte, n+16)
			response = append(requestID, buff[:n]...)
			go handleLocalResponse(requestID, response)
			return
		}
	}

}

func handleLocalResponse(requestID []byte, data []byte) {
	log.Printf("本地响应：%x", data[:16])
	proxyTunnel.ClientConn.Write(data)
	//关闭本地连接
	closeLocalConn(requestID)
}

func closeLocalConn(requestID []byte) {
	if conn, ok := proxySession[string(requestID)]; ok {
		conn.Close()
		delete(proxySession, string(requestID))
		log.Printf("关闭本地连接：%x", requestID)
	}
}
