package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"github.com/wowoniu/go_test/proxy/protocol"
)

type Tunnel struct {
	ClientConn     net.Conn
	ClientResponse chan []byte
}

type Request struct {
	CreateTime time.Time
	Conn       net.Conn
}

var (
	proxyTunnel    *Tunnel
	clientPort     string
	proxyPort      string
	proxySession   map[string]*Request
	buffSize       int   = 1024
	requestNum     int64 = 0
	tmpData              = make([]byte, 0)
	requestExpired int   = 15
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU() * 4)
}

func main() {
	flag.StringVar(&clientPort, "clientPort", "9000", "客户端连接的端口")
	flag.StringVar(&proxyPort, "proxyPort", "8082", "代理端口")
	flag.Parse()
	proxyTunnel = &Tunnel{
		ClientResponse: make(chan []byte, 1000),
	}
	proxySession = make(map[string]*Request)

	defer func() {
		for _, proxyRequest := range proxySession {
			proxyRequest.Conn.Close()
		}
		closeClientConn()
	}()

	go handleClientResponse()
	go listenClient()
	go listenProxyRequest()
	//go closeExpiredRequestConn()

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
				//声明一个临时缓冲区，用来存储被截断的数据
				tmpBuffer := make([]byte, 0)
				for {
					buff := make([]byte, buffSize)
					data := make([]byte, 0)
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
					//响应解包
					tmpBuffer, data = protocol.Unpack(append(tmpBuffer, buff[:n]...))
					if len(data) > 0 {
						pushClientResponse(data)
					} else {
						log.Println("无效的响应")
					}

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
		//log.Println("等待代理请求")
		if requestConn, err := requestListener.Accept(); err != nil {
			log.Println("proxy request connect error:", err)
			continue
		} else {
			//开启协程 读取代理请求的数据
			go func(conn net.Conn) {
				//defer conn.Close()
				requestID := createRequestUUID()
				requestIDStr := fmt.Sprintf("%x", requestID)
				proxySession[requestIDStr] = &Request{
					CreateTime: time.Now(),
					Conn:       conn,
				}
				log.Println("代理请求连接建立:", requestIDStr)
				n := 1
				for {
					buff := make([]byte, 1024)
					n, err = conn.Read(buff)
					if err != nil {
						//请求连接关闭
						if err == io.EOF {
							log.Println("代理请求数据读取完毕:", requestIDStr)
							log.Println("REQUEST END:", string(buff[:n]))
							return
						} else {
							log.Println("代理请求连接已断开:", err)
							closeRequestConn(requestID[:])
							return
						}
					} else {
						//处理请求
						data := append(requestID[:], buff[:n]...)
						log.Printf("接收到请求:%s", requestIDStr)
						// log.Println("REQUEST:")
						// log.Println(string(buff[:n]))
						go handleProxyRequest(data)
					}

				}
			}(requestConn)
		}
	}

}

func handleProxyRequest(data []byte) {
	// contentLength := getHTTPContentLength(data)
	// log.Println("响应长度：", contentLength)
	if proxyTunnel.ClientConn != nil {
		log.Println("代理请求 转发至客户端连接")
		// log.Println("代理请求封包:")
		// log.Println(string(protocol.Packet(data)))
		//proxyTunnel.ClientConn.Write(data)
		proxyTunnel.ClientConn.Write(protocol.Packet(data))
	} else {
		log.Println("没有有效的客户端连接")
	}

}

func handleClientResponse() {
	for {
		data := <-proxyTunnel.ClientResponse
		//前16个字节长度为requestID
		requestID := data[:16]
		responseData := data[16:]
		//log.Printf("客户端响应:%x\n", requestID)
		if string(responseData) == "CLOSE" {
			log.Printf("OVER:%s\n", responseData)
			closeRequestConn(requestID)
		} else {
			if proxyRequest, ok := proxySession[fmt.Sprintf("%x", requestID)]; ok {
				tmpData = append(tmpData, responseData...)
				//log.Printf("客户端响应写入(%x):%v", requestID, len(responseData))
				proxyRequest.Conn.Write(responseData)
			} else {
				//log.Printf("请求连接已断开 响应失败:%x", requestID)
			}
		}
	}
}

func closeRequestConn(requestID []byte) {
	log.Printf("代理请求关闭...:%x", requestID)
	requestIDStr := fmt.Sprintf("%x", requestID)
	if proxyRequest, ok := proxySession[requestIDStr]; ok {
		proxyRequest.Conn.Close()
		delete(proxySession, requestIDStr)
		log.Printf("代理请求结束:%s", requestIDStr)
	}
}

func closeClientConn() {
	log.Println("关闭客户端连接")
	if proxyTunnel.ClientConn != nil {
		proxyTunnel.ClientConn.Close()
		proxyTunnel.ClientConn = nil
	}
}

func closeExpiredRequestConn() {
	ticker := time.Tick(time.Second * time.Duration(requestExpired))
	for {
		select {
		case <-ticker:
			log.Println("超时GC启动")
			for requestIDStr, proxyRequest := range proxySession {
				if time.Now().Sub(proxyRequest.CreateTime) > time.Duration(requestExpired)*time.Second {
					log.Println("关闭超时连接:", requestIDStr)
					proxyRequest.Conn.Close()
					delete(proxySession, requestIDStr)
				}
			}
		}
	}

}

func createRequestUUID() []byte {
	requestNum++
	requestID := md5.Sum([]byte(fmt.Sprintf("%v%v%v", time.Now().UnixNano(), rand.Intn(1000), requestNum)))
	return requestID[:]
	// requestIDStr := fmt.Sprintf("%x", requestID[:])
	// log.Printf("生成请求ID：%s", requestIDStr)
	// return []byte(requestIDStr)
}

func getHTTPContentLength(httpContent []byte) int {
	//host替换
	reg := regexp.MustCompile(`Content-Length: (\d+)\r\n`)
	subFind := reg.FindSubmatch(httpContent)
	if subFind != nil {
		contentLength := subFind[1]
		if contentLengthNum, err := strconv.Atoi(string(contentLength)); err != nil {
			return 0
		} else {
			return contentLengthNum
		}
	}

	return 0
}

func pushClientResponse(response []byte) {
	// data := make([]byte, len(response))
	// copy(data, response)
	proxyTunnel.ClientResponse <- response
	//log.Println("收到结果:")
}
