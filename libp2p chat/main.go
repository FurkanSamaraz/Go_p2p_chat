package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"os"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"

	"github.com/multiformats/go-multiaddr"
)

func handleStream(s network.Stream) {
	log.Println("Yeni bir akış var!")

	// Engellemeyen okuma ve yazma için bir arabellek akışı oluşturun.
	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	go readData(rw)
	go writeData(rw)

	// stream 's' siz kapatana kadar (veya diğer taraf kapatana kadar) açık kalacaktır.
}

func readData(rw *bufio.ReadWriter) {
	for {
		str, _ := rw.ReadString('\n')

		if str == "" {
			return
		}
		if str != "\n" {
			// Green console colour: 	\x1b[32m
			// Reset console colour: 	\x1b[0m
			fmt.Printf("\x1b[32m%s\x1b[0m> ", str)
		}

	}
}

func writeData(rw *bufio.ReadWriter) {
	stdReader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')
		if err != nil {
			log.Println(err)
			return
		}

		rw.WriteString(fmt.Sprintf("%s\n", sendData))
		rw.Flush()
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sourcePort := flag.Int("sp", 0, "Kaynak bağlantı noktası numarası")
	dest := flag.String("d", "", "Hedef multiaddr dizesi")
	help := flag.Bool("help", false, "Yardımcıları göster")
	debug := flag.Bool("debug", false, "Hata ayıklama, her yürütmede aynı düğüm kimliğini oluşturur")

	flag.Parse()

	if *help {
		fmt.Printf("Bu program, libp2p kullanan basit bir p2p sohbet uygulamasını gösterir.\n\n")
		fmt.Println("Kullanım: './chat -app <SOURCE PORT>' komutunu çalıştırın, burada <SOURCE PORT> herhangi bir port numarası olabilir.")
		fmt.Println("Şimdi './chat -d <MULTIADDR>' komutunu çalıştırın, burada <MULTIADDR> önceki dinleyici ana bilgisayarının çoklu adresidir.")

		os.Exit(0)
	}

	// Hata ayıklama etkinse, eş kimliğini oluşturmak için sabit bir rastgele kaynak kullanın. Yalnızca hata ayıklama için kullanışlıdır,
	// varsayılan olarak kapalı. Aksi takdirde, Rand.Reader'ı kullanır.
	var r io.Reader
	if *debug {
		// Bağlantı noktası numarasını rastgelelik kaynağı olarak kullanın.
		// Bu, aynı bağlantı noktası numarası kullanılıyorsa, birden çok yürütmede her zaman aynı ana bilgisayar kimliğini oluşturur.

		r = mrand.New(mrand.NewSource(int64(*sourcePort)))
	} else {
		r = rand.Reader
	}

	h, err := makeHost(*sourcePort, r)
	if err != nil {
		log.Println(err)
		return
	}

	if *dest == "" {
		startPeer(ctx, h, handleStream)
	} else {
		rw, err := startPeerAndConnect(ctx, h, *dest)
		if err != nil {
			log.Println(err)
			return
		}

		// Verileri okumak ve yazmak için bir iş parçacığı oluşturun.
		go writeData(rw)
		go readData(rw)

	}

	// Sonsuza kadar bekle
	select {}
}

func makeHost(port int, randomness io.Reader) (host.Host, error) {
	// Bu ana bilgisayar için yeni bir RSA anahtar çifti oluşturur.
	prvKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, randomness)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	// 0.0.0.0 herhangi bir arayüz cihazında dinleyecektir.
	sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port))

	// libp2p.New, yeni bir libp2p Ana Bilgisayarı oluşturur.

	return libp2p.New(
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.Identity(prvKey),
	)
}

func startPeer(ctx context.Context, h host.Host, streamHandler network.StreamHandler) {

	// Bu işlev, bir eş bağlandığında çağrılır ve bu protokolle bir akış başlatır.
	// Sadece alıcı tarafta geçerlidir.
	h.SetStreamHandler("/chat/1.0.0", streamHandler)

	// 0 (varsayılan; rastgele kullanılabilir bağlantı noktası) kullanmamız durumunda, dinleme multiaddr'ımızdan gerçek TCP bağlantı noktasını alalım.
	var port string
	for _, la := range h.Network().ListenAddresses() {
		if p, err := la.ValueForProtocol(multiaddr.P_TCP); err == nil {
			port = p
			break
		}
	}

	if port == "" {
		log.Println("port yerel bağlantı noktasını bulamadı")
		return
	}

	log.Printf("Run './main -d /ip4/127.0.0.1/tcp/%v/p2p/%s' başka bir konsoldan erişiniz.\n", port, h.ID().Pretty())
	log.Println("127.0.0.1'i genel IP ile de değiştirebilirsiniz.")
	log.Println("Gelen bağlantı bekleniyor")
	log.Println()
}

func startPeerAndConnect(ctx context.Context, h host.Host, destination string) (*bufio.ReadWriter, error) {
	log.Println("Bu düğümün çoklu adresleri:")
	for _, la := range h.Addrs() {
		log.Printf(" - %v\n", la)
	}
	log.Println()

	// Hedefi bir multiaddr'ye çevirin.
	maddr, err := multiaddr.NewMultiaddr(destination)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	// Eş kimliğini multiaddr'den çıkarın.
	info, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	h.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.PermanentAddrTTL)

	// Hedefle bir akış başlatın.
	// Hedef eşin Çoklu Adresi, 'peerId' kullanılarak eş depodan alınır.
	s, err := h.NewStream(context.Background(), info.ID, "/chat/1.0.0")
	if err != nil {
		log.Println(err)
		return nil, err
	}
	log.Println("Established connection to destination")

	// Okuma ve yazma işlemlerinin engellenmemesi için arabelleğe alınmış bir akış oluşturun.
	rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	return rw, nil
}
