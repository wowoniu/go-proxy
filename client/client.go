package main

import (
	"flag"
	"log"
	"net"
)

type Tunnel struct {
	AppID      string
	ClientConn net.Conn
	LocalConn  net.Conn
}

var (
	proxyTunnel *Tunnel
	serverAddr  string
	proxyPort   string
)

func main() {
	flag.StringVar(&serverAddr, "serverAddr", "127.0.0.1:9000", "代理服务器地址")
	flag.StringVar(&proxyPort, "proxyPort", "80", "代理端口")
	go connectToServer()
	for {
	}
}

func connectToServer() {
	tcpAddr, _ := net.ResolveTCPAddr("tcp4", serverAddr)
	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		log.Fatal("connected error :", err)
	}
	defer conn.Close()
	log.Println("server connect success")
	proxyTunnel = &Tunnel{
		ClientConn: conn,
	}

	buff := make([]byte, 512)
	for {
		if n, err := conn.Read(buff); err != nil {
			log.Fatal("READ FROM SERVER ERROR:", err)
		} else {
			go handleRequest(buff[:n])
		}
	}

}

func handleRequest(data []byte) {
	if proxyTunnel.LocalConn == nil {
		//建立本地连接
		tcpAddr, _ := net.ResolveTCPAddr("tcp4", "127.0.0.1:"+proxyPort)
		conn, err := net.DialTCP("tcp", nil, tcpAddr)
		if err != nil {
			log.Println("connected error :", err)
			return
		}
		defer conn.Close()
		log.Println("server connect success")
		proxyTunnel.LocalConn = conn

		buff := make([]byte, 512)
		go func() {
			for {
				if n, err := conn.Read(buff); err != nil {
					log.Println("READ FROM LOCAL ERROR:", err)
					return
				} else {
					go handleLocalResponse(buff[:n])
				}
			}
		}()
	}
	proxyTunnel.LocalConn.Write(data)
}

func handleLocalResponse(data []byte) {

}
