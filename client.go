package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

func segmentString(userString string) [][]byte {

	var segments [][]byte /* buffer to store user input in 2-byte segments */

	for len(userString) > 0 {
		if len(userString) > 2 {
			segments = append(segments, []byte(userString[:2])) /* tacks on 2-byte segments of the string into the buffer */
			userString = userString[2:] /* removes the iterated characters from the string */
		} else {
			segments = append(segments, []byte(userString)) /* appends the last 1-2 remaining elements to the segment buffer */
			userString = ""
		}
	}

	return segments
}

func sendSegment(seqNumber int, stringSegment []byte, buffer []byte, socket *net.UDPConn) {

	packet := fmt.Sprintf("%11d%4d", seqNumber, len(stringSegment))
	copy(buffer, packet)
	copy(buffer[len(packet):], stringSegment)

	_, err := socket.Write(buffer)
	if err != nil {
		log.Fatalf("Segment send failure: %q\n", err)
	}
}


func main() {	

	/* declarations/initializations */
	var userString string /* used to store user string */
	inputReader := bufio.NewReader(os.Stdin) /* used to read user input w/ whitespaces */
	buffer := make([]byte, 17) /* buffer for sending string and string size */
	ackBuffer := make([]byte, 11) /* buffer to store ACK from server */
	ackChannel := make(chan int) /* channel used to send ACK back to main() */
	var ackNumber int
	const windowSize int = 5 /* 5 instead of 10 since data will be sent in 2-byte segments (5 segment max window size)*/
	
	if len(os.Args) > 3 {
		log.Fatalf("Usage is: ./client <server_ip> <server_port>\n") /* improper command line arg formatting */
	}

	serverAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(os.Args[1], os.Args[2])) /* where os.Args[1] = server ip */
	if err != nil {
		log.Fatalf("Entered IP address is invalid: %q\n", err)
	}

	udpSocket, err := net.DialUDP("udp", nil, serverAddr) /* second arg = nil means os automatically assigns port number */
	if err != nil {
		log.Fatalf("Failure setting up socket: %q\n", err)
	}
	defer udpSocket.Close() /* waits to close socket until end of program */
	

	fmt.Printf("Enter string to send to %s:%s : ", os.Args[1], os.Args[2]) /* prompts user to enter string and displays server IP and port number */
	userString, err = inputReader.ReadString('\n')
	if err != nil {
		log.Fatalf("Error with user input: %q\n", err)
	}

	userString = strings.TrimSuffix(userString, "\n") /* gets rid of trailing newline char */

	binary.BigEndian.PutUint32(buffer, uint32(len(userString))) /* writes the length of the string to the lengthBuffer in network order (big endian) */

	bytesWritten, err := udpSocket.Write(buffer[:4]) /* writes string length to socket "WE WILL ASSUME IT GETS TO THE SERVER" */
	if err != nil {
		log.Fatalf("Error when writing string length: %q\n", err)
	}
	if bytesWritten != 4 {
		fmt.Printf("Only wrote %d length bytes (non-fatal)\n", bytesWritten)
	}

	go func() { /* goroutine to listen for server ACKS */
		for {
			bytesRead, _, err := udpSocket.ReadFromUDP(ackBuffer)
			if err != nil {
				fmt.Printf("Failed to read ack from server (non-fatal): %q\n", err)
			}
			fmt.Sscanf(string(ackBuffer[:bytesRead]), "%11d", &ackNumber)
			ackChannel <- ackNumber
		}
	}()

	stringSegments := segmentString(userString)
	
	udpSocket.SetReadDeadline(time.Now().Add(5 * time.Second))

    windowStartSeqNumber := 0
    // for windowStartSeqNumber < len(stringSegments) {
    //     /* send segments up to window size */
    //     for i := windowStartSeqNumber; i < windowStartSeqNumber + windowSize && i < len(stringSegments); i++ {
    //         sendSegment(nextSeqNumber, stringSegments[i], buffer, udpSocket)
    //         nextSeqNumber += 2
    //     }
	// }

	for windowStartSeqNumber < len(stringSegments) {
		select {
		case ackReceived := <-ackChannel:
			if ackReceived == windowStartSeqNumber {
				windowStartSeqNumber += 2
			}
	
		case <-time.After(time.Second * 5): /* timeout after 2 seconds */
			/* Resend packets starting from windowStartSeqNumber */
			fmt.Printf("Resending packets\n")
			for i := windowStartSeqNumber; i < windowStartSeqNumber + windowSize && i < len(stringSegments); i++ {
				sendSegment(i * 2, stringSegments[i], buffer, udpSocket)
			}
		}
	}

}