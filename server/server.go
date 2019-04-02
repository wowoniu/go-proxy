package main

import (
	"flag"
	"log"
	"net"
)

type Tunnel struct {
	AppID       string
	RequestConn net.Conn
	ClientConn  net.Conn
}

var (
	proxyTunnel *Tunnel
	clientPort  string
	proxyPort   string
)

func main() {
	flag.StringVar(&clientPort, "clientPort", "9000", "客户端连接端口")
	flag.StringVar(&proxyPort, "proxyPort", "80", "代理端口")
	go listenClient()
	go listenProxyRequest()
	for {
	}
}

func listenClient() {
	//客户端连接监听
	clientListener, err := net.Listen("tcp", ":"+clientPort)
	if err != nil {
		log.Fatal("client connect port listen error:", err)
	}
	defer clientListener.Close()
	buf := make([]byte, 10240)
	for {
		conn, err := clientListener.Accept()
		if err != nil {
			//连接建立失败
			log.Println("client connect error:", err)
			continue
		}
		//客户端连接建立成功则创建成功一个TUNNEL
		proxyTunnel = &Tunnel{
			AppID:      "",
			ClientConn: conn,
		}
		//开启协程 读取客户端的响应数据
		go func() {
			buff := make([]byte, 10240)
			for {
				n, err := conn.Read(buff)
				if err != nil {
					//读取失败
					log.Println("client response read error:", err)
					proxyTunnel.ClientConn = nil
					return
				}
				//对响应进行转发处理
				go handleClientResponse(buf[:n])
			}
		}()
	}

}

func listenProxyRequest() {
	//代理请求监听
	requestListener, err := net.Listen("tcp", ":"+proxyPort)
	if err != nil {
		log.Fatal("request proxy port listen error:", err)
	}
	defer requestListener.Close()
	//等待代理请求建立连接
	for {
		requestConn, err := requestListener.Accept()
		if err != nil {
			log.Println("proxy request connect error:", err)
			continue
		}
		//开启协程 读取代理请求的数据
		go func() {
			buff := make([]byte, 10240)
			for {
				n, err := requestConn.Read(buff)
				if err != nil {
					//连接可能关闭
					log.Println("proxy request read error:", err)
					proxyTunnel.RequestConn = nil
					return
				}
				//处理代理请求的数据
				go handleProxyRequest(buff[:n])

			}
		}()

	}

}

func handleClientResponse(data []byte) {
	proxyTunnel.RequestConn.Write(data)
}

func handleProxyRequest(data []byte) {
	proxyTunnel.ClientConn.Write(data)
}
