package main

import (
	"context"         // for grpc
	"crypto/sha256"   // required for Hash
	"encoding/binary" // output of hash
	"flag"            // for command-line input
	"fmt"             // for printing
	"io/ioutil"
	"log" // to log the errors
	"math"
	"math/rand"
	"net"  // for server to listen
	"sync" // for mutex
	"time"

	"google.golang.org/grpc" // for rpc calls
	"gopkg.in/yaml.v3"

	pb "token_manage/proto" // import the generated protobuf code
)

// Token struct with the required properties
type Token struct {
	ID     string
	Name   string
	Domain struct {
		Low  uint64
		Mid  uint64
		High uint64
	}
	State struct {
		Partial uint64
		Final   uint64
	}
	Writer    string   // Writer Accesspoint
	Readers   []string // Reader Accesspoint
	Timestamp int64    // Timestamp of Final Value updates.
}

type TokenConfig struct {
	Id      string   `yaml:"Id"`      // Reads and stores yaml id
	Writer  string   `yaml:"Writer"`  // reads and stores yaml Writer accesspoint
	Readers []string `yaml:"Readers"` // reads and stores yaml Reader accesspoint
}

type Config struct {
	Tokens []TokenConfig // Stores all the tokens of TokenConfig format.
}

// TokenManagerServer is a structure given as an input to all the function
// This structure has the mutex which maintaint the management of shared resources.
type TokenManagerServer struct {
	pb.UnimplementedTokenManagerServer
	tokens      map[string]*Token // map of tokens to store all tokens
	accesspoint string            // accesspoint of that server to identify it
	mu          sync.RWMutex      // Sharing of resources
}

// Hash concatenates a message and a nonce and generates a hash value.
func Hash(name string, nonce uint64) uint64 {
	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("%s %d", name, nonce)))
	return binary.BigEndian.Uint64(hasher.Sum(nil))
}

// NewTokenManager creates a new instance of TokenManager
// required for grpc registering
func NewTokenManager(accesspoint string) *TokenManagerServer {
	return &TokenManagerServer{
		tokens:      make(map[string]*Token),
		accesspoint: accesspoint,
	}
}

func main() {
	var port = flag.Int("port", 50051, "The server port")
	// flag input get Int pointer, with flag name port, default value 50051 and description

	flag.Parse() // execute the parsing

	port_no := fmt.Sprintf(":%d", *port) // get the value of port and convert to string
	// : => the colon means on all available network interfaces

	lis, err := net.Listen("tcp", port_no) // Make the listening port
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	accesspoint := fmt.Sprintf("10.0.2.15:%d", *port) // store accesspoint

	file, err := ioutil.ReadFile("config.yaml") // Read the YAML file
	if err != nil {
		fmt.Errorf("failed to read: %v", err)
	}

	var config Config
	err = yaml.Unmarshal(file, &config) // Unpack the YAML file in config of struct Config defined above.
	if err != nil {
		fmt.Errorf("failed to unmarshal: %v", err)
	}

	tokenmanager := NewTokenManager(accesspoint) // New Token Manager Created with the respected accesspoint
	now := time.Now()

	for _, tokenConfig := range config.Tokens {

		// Token creation
		token := Token{
			ID:        tokenConfig.Id,
			Writer:    tokenConfig.Writer,
			Readers:   tokenConfig.Readers,
			Timestamp: now.Unix(), // Timestamp of token creation
		}

		tokenmanager.tokens[token.ID] = &token // Tokens are added to the tokenmanager
	}

	s := grpc.NewServer() // create instance of grpc server

	pb.RegisterTokenManagerServer(s, tokenmanager) // Register the Service with grpc server

	log.Printf("Starting server %d", *port) // Print after starting the server

	if err := s.Serve(lis); err != nil { // call server on server
		log.Fatalf("Failed to serve: %v", err)
	}

}

// Create creates a new token with the given ID
func (tm *TokenManagerServer) CreateToken(ctx context.Context, req *pb.CreateTokenRequest) (*pb.CreateTokenResponse, error) {
	tm.mu.Lock() // start the critical code
	defer tm.mu.Unlock()
	// defer tells to unlock the resource before leaving the function,
	//even if it leaves with an error

	id := req.GetId()               // get the requested ID
	if _, ok := tm.tokens[id]; ok { // if already present
		return nil, fmt.Errorf("token with id %s already exists", id)
	}

	//create new token
	tm.tokens[id] = &Token{
		ID: id,
	}

	tm.DumpToken(id) // print the token details at server side

	return &pb.CreateTokenResponse{ //return success message
		Success: true,
	}, nil

}

// Drop deletes the token with the given ID
func (tm *TokenManagerServer) DropToken(ctx context.Context, req *pb.DropTokenRequest) (*pb.DropTokenResponse, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	id := req.GetId() // get the id
	if _, ok := tm.tokens[id]; !ok {
		return nil, fmt.Errorf("token with id %s does not exist", id)
	}

	// delete the token from token structure

	delete(tm.tokens, id)

	// print the token details
	tm.DumpToken(id)

	// return success message
	return &pb.DropTokenResponse{
		Success: true,
	}, nil
}

// Write sets the properties of the token with the given ID
func (tm *TokenManagerServer) WriteToken(ctx context.Context, req *pb.WriteTokenRequest) (*pb.WriteTokenResponse, error) {

	id := req.GetId()      //get id
	t, ok := tm.tokens[id] // get token with the id
	if !ok {
		return nil, fmt.Errorf("token with id %s does not exist", id)
	}

	/*

		This code is used to emulate the fail-silent behaviour

		if id=="token1"{
			for{
				time.Sleep(time.Hour) // goes into infinite loop
			}
		}
	*/

	// Checks if the current server is the Writer of the Token.
	if tm.accesspoint != t.Writer {
		fmt.Printf("\nSending to Writer\n")

		// Setting connection with the Writer
		conn, err := grpc.Dial(t.Writer, grpc.WithInsecure())

		if err != nil {
			return nil, err
		}

		defer conn.Close() //Close connection before exiting the call

		client := pb.NewTokenManagerClient(conn) // Creating new Client

		resp, err := client.WriteToken(ctx, req) // Make Write Token call to the Writer

		if err != nil {
			log.Fatalf("Failed%v", err)
		}

		return &pb.WriteTokenResponse{ // return response
			Partial: resp.Partial,
		}, nil
	}

	fmt.Printf("\nI am Writer\n")

	readerNodes := t.Readers             // Get the reader nodes of token
	index := rand.Intn(len(readerNodes)) // Randomly select index of reader node
	reader := readerNodes[index]         // get the readernode at that index

	fmt.Println("\nReader:%v\n", reader) //print the selected reader

	conn, err := grpc.Dial(reader, grpc.WithInsecure()) // Create connection with the Reader

	if err != nil {
		return nil, err
	}

	defer conn.Close() // Close connection

	client := pb.NewTokenManagerClient(conn)

	_, err = client.ReadToken(ctx, &pb.ReadTokenRequest{Id: id}) // Send Read Token Request to the reader node.

	if err != nil {
		log.Fatalf("Failed%v", err)
	}

	fmt.Printf("\nBack to Writer\n")

	tm.mu.RLock() // Put Read Lock

	// Assign the Name, Low, Mid, High to the token
	t.Name = req.GetName()
	t.Domain.Low = req.GetLow()
	t.Domain.Mid = req.GetMid()
	t.Domain.High = req.GetHigh()

	// To find the min Hash Value
	var min uint64 = t.Domain.Low       // min index as low
	var minHash uint64 = math.MaxUint64 // Max possible integer

	for i := t.Domain.Low; i < t.Domain.High; i++ { // for range [low,mid)
		h := Hash(t.Name, i) // get hash
		if h < minHash {     // if hash is less then minHash
			min = i     // argmin
			minHash = h // new min hash
		}
	}

	now := time.Now()
	t.State.Final = min     // set the Partial Value
	timestamp := now.Unix() // get current timestamp

	t.Timestamp = timestamp //assign it to token
	tm.mu.RUnlock()         // Unlock the read lock

	impose_response := make(chan bool) // Create channel for response from RPC call

	for _, readernode := range readerNodes { // for every reader Node
		go func(readernode string) { // go routine
			conn, err := grpc.Dial(readernode, grpc.WithInsecure()) // Make connection
			if err != nil {
				impose_response <- false
				return
			}
			defer conn.Close()
			client := pb.NewTokenManagerClient(conn)
			res, err := client.ImposeValue(ctx, &pb.ImposeValueRequest{Id: id, Final: min, Timestamp: timestamp})
			// Send Impose Value request to all the reader nodes
			if err != nil {
				impose_response <- false
				return
			}

			impose_response <- res.GetSuccess() // store the response of all calls
		}(readernode)
	}

	// Check if all the rpc calls were a success.
	for i := 0; i < len(readerNodes); i++ {
		response := <-impose_response
		if response != true {
			return nil, fmt.Errorf("Write Token Failed")
		}
	}

	tm.DumpToken(id) // print token details

	return &pb.WriteTokenResponse{ // return response
		Partial: t.State.Final,
	}, nil

}

// Read calculates the final state of token
func (tm *TokenManagerServer) ReadToken(ctx context.Context, req *pb.ReadTokenRequest) (*pb.ReadTokenResponse, error) {

	// Find the token by id.
	id := req.GetId()
	token, exists := tm.tokens[id] // find token from id
	if !exists {
		return nil, fmt.Errorf("token with id %s does not exist", req.GetId())
	}

	// Get Reader Nodes of the token
	readerNodes := token.Readers
	accesspoint := tm.accesspoint

	// Check if the current server belongs to reader Nodes
	if !isReader(accesspoint, readerNodes) { //if not

		fmt.Printf("\nCalling the appropriate Reader\n")

		// Select random reader
		index := rand.Intn(len(readerNodes))
		reader := readerNodes[index]

		fmt.Printf("\nReader:%v\n", reader)

		// make connection with reader
		conn, err := grpc.Dial(reader, grpc.WithInsecure())

		if err != nil {
			return nil, err
		}

		defer conn.Close()

		client := pb.NewTokenManagerClient(conn)

		// Forward the Read request to appropriate reader
		res, err := client.ReadToken(ctx, &pb.ReadTokenRequest{Id: id})

		if err != nil {
			log.Fatalf("Failed%v", err)
		}

		// return the Final Value to the client
		return &pb.ReadTokenResponse{Final: res.Final}, nil

	}

	fmt.Printf("\nI am the Reader\n")

	// Make channel for response from rpc calls
	latest_values := make(chan *pb.SendLatestResponse)

	for _, readernode := range readerNodes { // Request all reader nodes
		tm.mu.RLock()
		go func(readernode string) { // go routine for concurrent calls
			fmt.Printf("\nAsking for latest response from %v\n", readernode)
			conn, err := grpc.Dial(readernode, grpc.WithInsecure())
			if err != nil {
				latest_values <- nil
				return
			}
			defer conn.Close()
			client := pb.NewTokenManagerClient(conn)
			// Make Send Latest Call to all the reader Nodes
			res, err := client.SendLatest(ctx, &pb.SendLatestRequest{Id: id})
			if err != nil {
				latest_values <- nil
				return
			}

			latest_values <- res
		}(readernode)
		tm.mu.RUnlock()
	}

	// Gets the value and timestamp in the readers copy.
	this_value := token.State.Final
	this_timestamp := token.Timestamp

	impose_needed := 0 // flag to check if impose is needed

	for i := 0; i < len(readerNodes); i++ {
		res := <-latest_values // gets the response from channel
		value := res.Final     // unpacks the response
		timestamp := res.Timestamp

		if timestamp > this_timestamp && this_value != value { // check if other timestamps are latest
			this_value = value
			this_timestamp = timestamp
			impose_needed = 1
		} else if this_value != value { // if the current readers timestamp is latest, but the values are different
			impose_needed = 1
		}
	}

	if impose_needed == 1 { // If an impose is required

		fmt.Printf("\nImposing Final Value")

		// Save the latest value and timestamp
		token.State.Final = this_value
		token.Timestamp = this_timestamp

		impose_response := make(chan bool) // Create channel for impose response

		for _, readernode := range readerNodes {
			tm.mu.RLock()
			go func(readernode string) {
				conn, err := grpc.Dial(readernode, grpc.WithInsecure())
				if err != nil {
					impose_response <- false
					return
				}
				defer conn.Close()
				client := pb.NewTokenManagerClient(conn)
				res, err := client.ImposeValue(ctx, &pb.ImposeValueRequest{Id: id, Final: this_value, Timestamp: this_timestamp})
				if err != nil {
					impose_response <- false
					return
				}

				impose_response <- res.GetSuccess()
			}(readernode)
			tm.mu.RUnlock()
		}

		// Check if all the Imposes are done succesfully
		for i := 0; i < len(readerNodes); i++ {
			response := <-impose_response
			if response != true {
				return nil, fmt.Errorf("Impose Failed")
			}
		}

	}

	tm.DumpToken(id) // print token details

	//return final value as response
	return &pb.ReadTokenResponse{Final: token.State.Final}, nil
}

func (tm *TokenManagerServer) SendLatest(ctx context.Context, req *pb.SendLatestRequest) (*pb.SendLatestResponse, error) {

	tm.mu.RLock()
	defer tm.mu.RUnlock()

	fmt.Printf("\n I am sending latest value\n")

	// Find the token by id.
	id := req.GetId()
	token, exists := tm.tokens[id] // find token from id
	if !exists {
		return nil, fmt.Errorf("token with id %s does not exist", req.GetId())
	}

	// Get the finalvalue and timestamp
	final := token.State.Final
	timestamp := token.Timestamp

	// send it as a response
	return &pb.SendLatestResponse{Final: final, Timestamp: timestamp}, nil
}

func (tm *TokenManagerServer) ImposeValue(ctx context.Context, req *pb.ImposeValueRequest) (*pb.ImposeValueResponse, error) {

	tm.mu.Lock()
	defer tm.mu.Unlock()

	fmt.Printf("\nValue is being imposed\n")

	// Find the token by id.
	id := req.GetId()

	// Get the value to be imposed
	imposevalue := req.GetFinal()
	imposetimestamp := req.GetTimestamp()

	token, exists := tm.tokens[id] // find token from id
	if !exists {
		return nil, fmt.Errorf("token with id %s does not exist", req.GetId())
	}

	timestamp := token.Timestamp
	if imposetimestamp > timestamp { // check if the imposed values timestamp is latest
		token.State.Final = imposevalue
		token.Timestamp = imposetimestamp

	} else {
		return &pb.ImposeValueResponse{Success: false}, nil // if the impose timestamp is not latest, send error
	}

	return &pb.ImposeValueResponse{Success: true}, nil
}

// Dump all the token info after rpc calls on stdout
func (tm *TokenManagerServer) DumpToken(id string) {

	t, ok := tm.tokens[id] //get token from id

	if !ok {
		fmt.Errorf("token with id %s does not exist", id)
	} else { // Print the details of that token
		fmt.Println("\n RPC ended for: \n Token Id: ", t.ID)
		fmt.Println("Token Reader: ", t.Readers)
		fmt.Println("Token Writer: ", t.Writer)
		fmt.Println("Token State Final Value: ", t.State.Final)
		fmt.Println("Token latest Timestamp: ", t.Timestamp)
	}

	// Print the id's of all tokens
	fmt.Println("\nAll Token id's:")
	for id := range tm.tokens {
		fmt.Println(id)
	}

	// return back to original function
	return
}

// Function to check if current server belongs to Reader of Token.
func isReader(accesspoint string, readerNodes []string) bool {
	for _, node := range readerNodes {
		if node == accesspoint {
			return true
		}
	}
	return false
}
