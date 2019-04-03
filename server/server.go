package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"
)

type Tunnel struct {
	ClientConn net.Conn
}

var (
	proxyTunnel  *Tunnel
	clientPort   string
	proxyPort    string
	proxySession map[string]net.Conn
	buffSize     int = 102400
)

func main() {
	flag.StringVar(&clientPort, "clientPort", "9000", "客户端连接的端口")
	flag.StringVar(&proxyPort, "proxyPort", "8082", "代理端口")
	proxyTunnel = &Tunnel{}
	proxySession = make(map[string]net.Conn)

	defer func() {
		for _, conn := range proxySession {
			conn.Close()
		}
		closeClientConn()
	}()
	go listenClient()
	go listenProxyRequest()
	for {
	}
}

func listenClient() {
	//客户端连接监听
	log.Println("监听客户端端口:", clientPort)
	clientListener, err := net.Listen("tcp", ":"+clientPort)
	if err != nil {
		log.Fatal("client connect port listen error:", err)
	}
	defer clientListener.Close()
	for {
		log.Println("等待客户端连接")
		if conn, err := clientListener.Accept(); err != nil {
			//连接建立失败
			log.Println("client connect error:", err)
			continue
		} else {
			//客户端连接建立成功则创建成功一个TUNNEL
			fmt.Println("客户端连接成功")
			proxyTunnel.ClientConn = conn
			//开启协程 处理客户端数据通信
			clientClosed := make(chan bool)
			go func(clientClosed chan bool) {
				for {
					buff := make([]byte, buffSize)
					n, err := conn.Read(buff)
					if err != nil {
						//读取失败 端口与客户端的连接
						log.Println("client response read error:", err)
						if proxyTunnel.ClientConn != nil {
							proxyTunnel.ClientConn.Close()
							proxyTunnel.ClientConn = nil
						}
						clientClosed <- true
						return
					}
					//对响应进行转发处理
					// fmt.Println(string(buff[:n]))
					go handleClientResponse(buff[:n])
				}
			}(clientClosed)
			<-clientClosed
		}

	}

}

func listenProxyRequest() {
	//代理请求监听
	requestListener, err := net.Listen("tcp", ":"+proxyPort)
	if err != nil {
		log.Fatal("request proxy port listen error:", err)
	}
	defer requestListener.Close()
	log.Println("监听代理端口:", proxyPort)
	//等待代理请求建立连接
	for {
		log.Println("等待代理请求")
		if requestConn, err := requestListener.Accept(); err != nil {
			log.Println("proxy request connect error:", err)
			continue
		} else {
			//开启协程 读取代理请求的数据
			go func(conn net.Conn) {
				requestID := md5.Sum([]byte(fmt.Sprintf("%v%v", time.Now().UnixNano(), rand.Intn(1000))))
				proxySession[string(requestID[:])] = conn
				log.Println("代理请求连接建立:", fmt.Sprintf("%x", requestID))
				for {
					buff := make([]byte, 1024)
					n, err := conn.Read(buff)
					if err != nil {
						//请求连接关闭
						log.Println("proxy request read error:", err)
						closeRequestConn(requestID[:])
						return
					}
					//处理代理请求的数据
					data := make([]byte, n+16)
					data = append(requestID[:], buff[:n]...)
					log.Printf("接收到请求:%x", requestID[:])
					go handleProxyRequest(data)

				}
			}(requestConn)
		}
	}

}

func handleClientResponse(data []byte) {
	//前16个字节长度为requestID
	requestID := data[:16]
	responseData := data[16:]
	fmt.Printf("客户端响应:%x\n", data[:16])
	if conn, ok := proxySession[string(requestID)]; ok {
		conn.Write(responseData)
		conn.Close()
		delete(proxySession, string(requestID))
	} else {
		log.Printf("请求连接已断开 响应失败:%x", requestID)
	}
}

func handleProxyRequest(data []byte) {
	if proxyTunnel.ClientConn != nil {
		log.Println("代理请求 写入客户端连接")
		// log.Println("代理请求内容:")
		// log.Println(string(data))
		proxyTunnel.ClientConn.Write(data)
	}

}

func closeRequestConn(requestID []byte) {
	log.Printf("请求连接关闭:%x", requestID)
	if conn, ok := proxySession[string(requestID)]; ok {
		conn.Close()
		delete(proxySession, string(requestID))
	}
}

func closeClientConn() {
	log.Println("关闭客户端连接")
	if proxyTunnel.ClientConn != nil {
		proxyTunnel.ClientConn.Close()
		proxyTunnel.ClientConn = nil
	}
}
