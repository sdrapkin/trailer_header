// Description: This Go program demonstrates how to use HTTP trailer headers
// to send metadata about the request body after the body has been sent.
// It includes a simple HTTP server that reads the request body and checks
// the trailer header for the body length. The client sends a request with
// a trailer header indicating the length of the body. The server verifies
// the length and logs the results. The program uses io.Pipe to stream
// the request body, allowing the client to send data without knowing
// the size in advance. The server and client run concurrently, and the
// program logs the interactions between them.

// HTTP Trailer Fields spec: https://www.rfc-editor.org/rfc/rfc9110.html#trailer.fields

package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

const trailerHeaderName = "X-Body-Byte-Length"

// serverHandler processes requests with potential trailer headers
func serverHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close() // Ensure the request body is closed
	log.Println("Server: Received request")
	log.Printf("Server: Request Method: %s", r.Method)

	// 1. Log initial request headers
	log.Println("Server: Initial Request Headers:")
	for name, values := range r.Header {
		fmt.Printf("  %s: %s\n", name, values)
	}

	// Check if the client announced a trailer header
	trailerHeaderNames := r.Header.Get("Trailer")
	log.Printf("Server: Announced Trailer header names: %s", trailerHeaderNames)

	// 2. Read the request body completely.
	// Trailer headers are only available *after* the body is fully read.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Server: Error reading request body: %v", err)
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	log.Printf("Server: Read request body (%d bytes): %s", len(body), string(body))
	calculatedBodyLength := len(body)

	// 3. Access the trailer headers from the request object.
	// This map is populated by the server *after* the body is read.
	log.Println("Server: Trailer Headers:")
	if len(r.Trailer) > 0 {
		for name, values := range r.Trailer {
			fmt.Printf("  %s: %s\n", name, values)
			// Process the specific trailer header we expect
			if name == trailerHeaderName && len(values) > 0 {
				trailerLengthStr := values[0]
				trailerLength, cerr := strconv.Atoi(trailerLengthStr)
				if cerr != nil {
					log.Printf("Server: Could not parse trailer length '%s': %v", trailerLengthStr, cerr)
				} else {
					log.Printf("Server: Trailer reported body length: %d bytes", trailerLength)
					// Verify the length
					if trailerLength == calculatedBodyLength {
						log.Println("Server: Body length matches trailer length. Integrity check successful!")
					} else {
						log.Println("Server: Body length DOES NOT match trailer length. Data integrity issue!")
					}
				}
			}
		}
	} else {
		log.Println("Server: No trailer headers received.")
	}

	// 4. Send a simple response back to the client
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Server received your request and processed trailers.\n")); err != nil {
		log.Printf("Server: Error writing response: %v", err)
	} else {
		log.Println("Server: Sent response")
	}
} // serverHandler() func

func main() {
	// Start the HTTP server in a goroutine
	go func() {
		http.HandleFunc("/", serverHandler)
		addr := "localhost:8080"
		log.Printf("Server: Starting on %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatalf("Server: Failed to start: %v", err)
		}
	}()

	// Give the server a moment to start
	time.Sleep(500 * time.Millisecond)

	// --- Client side ---
	log.Println("\nClient: Preparing request with trailer")

	// 1. Define the request body content
	requestBodyContent := "abcde"
	requestBodyBytes := []byte(requestBodyContent)
	requestBodyByteLength := len(requestBodyBytes)

	// 2. Create an io.Pipe. This allows streaming the body.
	pr, pw := io.Pipe()

	// 3. Create the HTTP request with the reader end of the pipe as the body.
	// This signals to the Go client that the body is being streamed
	// and its size is not known upfront, triggering chunked encoding.
	req, err := http.NewRequest("POST", "http://localhost:8080", pr)
	if err != nil {
		log.Fatalf("Client: Failed to create request: %v", err)
	}

	// 4. Set the initial Trailer header to announce the trailer header name
	req.Header.Set("Trailer", trailerHeaderName)

	// Go's http client will automatically handle Transfer-Encoding: chunked
	// when you provide a request body reader and don't set Content-Length,
	// and you have the Trailer header set.

	// 5. Set the actual trailer header value on the request's Trailer map
	req.Trailer = http.Header{} // Initialize the map
	req.Trailer.Set(trailerHeaderName, strconv.Itoa(requestBodyByteLength))

	log.Printf("Client: Sending request with body (%d bytes) and Trailer: %v", requestBodyByteLength, req.Trailer)

	// 6. Write the body content to the writer end of the pipe in a goroutine.
	// This allows the client.Do call (which reads from the reader end) in Step-7 to proceed concurrently.
	// Closing the writer signals the end of the body stream.
	go func() {
		log.Println("Client: Starting to write body to pipe")
		// Write the body content to the pipe writer
		_, writeErr := pw.Write(requestBodyBytes)
		if writeErr != nil {
			log.Printf("Client: Error writing body to pipe: %v", writeErr)
			// ClosingWithError causes the reader side (client.Do) to receive this error
			pw.CloseWithError(writeErr)
			return
		}
		log.Println("Client: Finished writing body to pipe")
		// **Crucially:** Close the pipe writer when done writing.
		// This signals the end of the body to the HTTP client.
		if err := pw.Close(); err != nil {
			log.Printf("Client: Error closing pipe writer: %v", err)
		}
	}()

	// 7. Send the request using the client.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Client: Failed to send request: %v", err)
	}
	defer resp.Body.Close()
	log.Printf("Client: Received response with status: %s", resp.Status)

	// Read the server's response body (optional, just for completeness)
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Client: Error reading response body: %v", err)
	} else {
		log.Printf("Client: Server response body: %s", string(responseBody))
	}

	log.Println("Client: Finished")
} // main
