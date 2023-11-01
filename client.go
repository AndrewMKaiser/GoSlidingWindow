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

	packet := fmt.Sprintf("%11d%4d", seqNumber, len(stringSegment)) /* copies header into buffer as shown in Canvas */
	copy(buffer, packet) /* copies the header into the buffer */
	copy(buffer[len(packet):], stringSegment) /* copies string data into the packet to be sent */

	_, err := socket.Write(buffer[:len(packet) + len(stringSegment)]) /* accounts for strings of odd length */
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
	var windowStartSeqNumber int = 0
	var windowEndSeqNumber int = windowSize * 2
	var nextSeqNumber int = 0
	
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
	userString, err = inputReader.ReadString('\n') /* reads all chars entered by user, including whitespace, up to and including newline char */
	if err != nil {
		log.Fatalf("Error with user input: %q\n", err)
	}

	userString = strings.TrimSuffix(userString, "\n") /* gets rid of trailing newline char */

	stringSegments := segmentString(userString) /* stores a slice of 2-character string segments */
 
	binary.BigEndian.PutUint32(buffer, uint32(len(userString))) /* writes the length of the string to the lengthBuffer in network order (big endian) */

	bytesWritten, err := udpSocket.Write(buffer[:4]) /* writes string length to socket "WE WILL ASSUME IT GETS TO THE SERVER" */
	if err != nil {
		log.Fatalf("Error when writing string length: %q\n", err)
	}
	if bytesWritten != 4 {
		fmt.Printf("Only wrote %d length bytes (non-fatal)\n", bytesWritten)
	}

	go func() { /* goroutine (thread) to listen for server ACKs */ 
		for {
			bytesRead, _, err := udpSocket.ReadFromUDP(ackBuffer)
			if err != nil {
				netErr, ok := err.(net.Error) /* asserts error to check for possible timeout - found at https://stackoverflow.com/questions/23494950/specifically-check-for-timeout-error */
            	if ok && netErr.Timeout() {
                	continue /* ignore if it's a timeout */ 
            	}
			}
			fmt.Sscanf(string(ackBuffer[:bytesRead]), "%11d", &ackNumber) /* moves server ACK into ackNumber */
			fmt.Printf("Received ACK: %d\n", ackNumber)

			if ackNumber == windowStartSeqNumber { /* slides window forward by 2 bytes if the ACK is the same as the window start index */
				windowStartSeqNumber += 2
				windowEndSeqNumber += 2
			}
			ackChannel <- ackNumber /* sends ACK down to main thread through ackChannel */
		}
	}()

	for nextSeqNumber < windowEndSeqNumber && nextSeqNumber / 2 < len(stringSegments) { /* initial send */
		sendSegment(nextSeqNumber, stringSegments[nextSeqNumber/2], buffer, udpSocket)
		nextSeqNumber += 2
	}
	
	
	for windowStartSeqNumber / 2 < len(stringSegments) { /* executes as long as there are segments left to send */

		udpSocket.SetReadDeadline(time.Now().Add(1 * time.Second)) /* timeout = 1 sec */
		
		select {
		case ackReceived := <-ackChannel: /* executes if ACK received from server */
			if ackReceived >= windowStartSeqNumber { /* slides window up to ACK */
				windowStartSeqNumber = ackReceived + 2 
				windowEndSeqNumber = windowStartSeqNumber + 10
				nextSeqNumber = windowStartSeqNumber
				fmt.Printf("Sending\n")
				if nextSeqNumber / 2 < len(stringSegments) {
					sendSegment(nextSeqNumber, stringSegments[nextSeqNumber / 2], buffer, udpSocket)
					nextSeqNumber += 2
				}
				udpSocket.SetReadDeadline(time.Time{})
			}
	
		case <-time.After(1 * time.Second): /* executes if timeout - resends all packets in window */
			if windowEndSeqNumber / 2 > len(stringSegments) {
				fmt.Printf("Resending packets #%d through #%d\n", windowStartSeqNumber / 2 + 1, len(stringSegments))
			} else {
				fmt.Printf("Resending packets #%d through #%d\n", windowStartSeqNumber / 2 + 1, windowEndSeqNumber / 2 + 1)
			}
			for i := windowStartSeqNumber; i < windowEndSeqNumber && i / 2 < len(stringSegments); i += 2 {
				sendSegment(i, stringSegments[i/2], buffer, udpSocket)
			}
			udpSocket.SetReadDeadline(time.Now().Add(1 * time.Second))
		}
	}
	

}
