# CMSC 621. Advanced Operating System.

# Project 3: Fail-Silent Replicated Token Manager with Atomic Semantics

## Bhargavi Poyekar (CH33454)
<br>

## Problem Statement: 

* You extend your client-server token manager from the last project to support replication and
atomic semantics.
* Each token is to be implemented as a (1,N) register with atomic semantics. 
* Your system should consist of a set of nodes that extend the server API from project 2. Your
solution should be developed in Go and utilize the gRPC and Google Protocol Buffer
frameworks.
* Extend each token to include the access points (IP address and port number) of its single writer
and multiple reader nodes; these nodes constitute the replication scheme of the token.
For simplicity, you may assume that the replication schemes of all tokens are static and known
apriori to all the server and client nodes in your system. Server nodes create tokens when they
start. Your implementation should use a simple YAML file with the replication scheme of all the
tokens, i.e. an array of

    token: <id>
    writer: <access-point>
    readers: array of <access-point>s

where <access-point> is of the form <ip-address>:<port>, whereas a writer may also be a reader.

* A client sends all its write requests for a token to the token’s writer node, while it sends its read
requests to any one of the token’s reader nodes (read requests for a token by the same client
may be sent to different reader nodes, eg. chosen at random). The single server node contacted
is the one responding to a client’s request, and any client can read/write any token.

* Extend the server API with additional RPC calls so that your system supports atomic semantics:
for any given token, whenever a client read returns a value v and a subsequent (by any)
client read returns a value w, then the write of w does not precede the write of v

* To this end, you may need to maintain additional state information about each token, and
implement the read-impose-write-all (quorum) protocol.

* Emulate fail-silent behavior for the server nodes as follows. A server node may “crash-stop” for
operations on a particular token X (for which it is a reader or writer) at some time t by indefinitely
postponing a response to all operations on X it receives after time t. A server may still be
responding as usual to requests on other tokens for which it is a reader or writer.

## Program Description:

* Proto files: 

    * My proto file is similar to last time. I have added more fields in the Token message and added more 
    rpc calls to implement the Read-Impose-Write-All Quorum protocol.
    
    * The message Token now consists of extra Reader, Writers and Timestamp fields. The Reader Field is added that passes the reader accesspoints of a token, while writer is for writer accesspoint. The timestamp is used to determine the latest value of a token.

    * I have added two more services SendLatest and ImposeValue. These services are for the Read-Impose-Write-All protocol. The Send Latest service request takes the id of the token and responds with the latest value and timestamp of the token. The ImposeValue request takes id, final value and timestamp and responds with a success true or false message. 
    
* Server code:

    * The Token struct is updated to add the Reader, Writer, and Timestamp field for the Read-Impose-Write-All protocol as mentioned above.
    * There is a TokenConfig and Config struct used for reading the YAML file of access points. 
    * I have added the accesspoint in the TokenManageServer which identifies the accesspoint of the particular servernode in 'IP:Port' pattern. This is used for restriction of access for server and reader nodes. 
    * I also changed the Mutex to RW Mutex, for fail-silent behaviour.
    * The NewTokenManager function is updated to take accesspoint as parameter and assign to the server when registering the servers.
    * The main function is updated with inclusion of reading the yaml file with the help of config struct and then creating the tokens while starting the servers with their respective accesspoints.
    * The server is registered with the new tokens created and then the server is started. 
    * Read-Impose-Write-All protocol:
        * The Writer Node sends read request to the Reader Nodes in reader quorum.
        * The Reader Nodes send their value with timestamp. 
        * The latest value is determined from the timestamp and that value and timestamp is imposed
        on all the nodes in quorum.
        * This ensures that all the nodes have same value before any update, since it is important to maintain consistency across all the nodes.
        * The Write function then calculates the new value and broadcasts that value to all the nodes 
        so that they can make their changes.  
        * When only Reading is required, the read node asks other reader nodes to send their values and timestamp, then compares it and if the timestamp of other node is more recent, it changes its own value and then asks other nodes to impose this value. 

    * Write Token RPC Call:

        * In this function, first I check if the current server is writer of the requested token, by checking the accesspoint of the server.
        * If not, then the request is sent to the writer node of the token by acting as a client and making grpc Write Token call to the respective writer node. The response that it receives from the grpc call is sent back to the client.
        * If it is the Writer, it checks all the reader nodes and selects a readernode randomly.
        A Read request is sent to this reader node, which then makes sure that all the nodes are on the same page. i.e. they all have the latest value.
        * Once the read response is received, the writer function calculates the final value according to the project 2, but here I am not using the Partial Value, since I am focusing on one value which is the Final Value and the Read will also check this value and not compute anything. 
        * The computation of Final Value is done with Mutex RLock, once the coputation is done, the Mutex. This helps in maintaining sequence in conflicting RPC calls.
        * Now the Writer Node makes concurrent grpc ImposeValue calls to other Reader Nodes to make update in value. These concurrent calls are done using goroutines. 
        * Once it receives success message from all the other reader nodes, the Write Token Call is completed and the Final Value is sent back to client. If any of the message sends failure message in updating their value, the Write Function sends an error message.

    * Read Token RPC Call:

        * The Read function first checks if the current server belongs to the readerNodes of token by comparing the accesspoints.
        * If it is not, a readernode is selected randomly from the reader Nodes of the token and the ReadToken request is sent to the selected readerNode using grpc Call and again the current server acts as a client in this situation.
        * If is a readerNode, the server uses go routines to make concurrent grpc SendLatest calls to all other reader Nodes. The Send Latest call responds with the values and timestamp maintained by other nodes for that token. 
        * Once all grpc responses are received, the Reader then compares te values and finds the latest value by comparing the timestamps. It decides if an impose is required. The impose is required if the value of other node differs from the value of current node.
        * If an impose is needed, it again uses go routines to make concurrent grpc Impose Value calls to Other reader nodes, so that all the nodes have same and latest value. 
        * Once it receives success from all the other nodes. It sends the Latest Value to the client.

    * Send Latest RPC Call:

        * This call finds the token from id, and then sends the final value and timestamp back to the reader. 

    * Impose Value RPC Call:
        * This call finds the token from id, and it also receives the value to be imposed and its timestamp. It compares the timestamp with its own timestamp and it the new timestamp is greater, it makes changes in its value and timestamp.
        * If the received timestamp is older, it sends an error message, whereas if everything goes correctly, it sends success message.

    * isReader: This is a helper function to check if the current server belongs to the tokens reader Nodes.

    * Fail-silent behavior:

        * To emulate this behavior, I added a small code in the server, which checks if the requested token is token1 and then it runs in infinite loop. 
        * WHen the client requests any server for operation on this token, it goes into infinite loop, and doesn't respond back. 
        * Then simulatneously I checked if another client can request operations on other tokens on that server, while the server is not responding to the first token. 
        * I have added a screenshot of this behaviour, named fail-silent.png It can also be seen in scripts provided, but since I had to run different server and clients on different terminals, the scripts are different for all the terminals.   

* Client code:

    * The client code is same as the project 2 one. As no changes were required in this. 

## Commands Executed:

* Create a YAML file with this format:

    tokens:
    - Id: "token1"
        Writer: "10.0.2.15:50051"
        Readers: ["10.0.2.15:50052","10.0.2.15:50053","10.0.2.15:50054"]
    - Id: "token2"
        Writer: "10.0.2.15:50052"
        Readers: ["10.0.2.15:50053","10.0.2.15:50054","10.0.2.15:50051"]
    - Id: "token3"
        Writer: "10.0.2.15:50054"
        Readers: ["10.0.2.15:50051","10.0.2.15:50052","10.0.2.15:50053"]
    - Id: "token4"
        Writer: "10.0.2.15:50053"
        Readers: ["10.0.2.15:50052","10.0.2.15:50054","10.0.2.15:50051"]


* Create tokens.proto file in this and run:

        protoc -I=$GOPATH/src --go_out=. --go-grpc_out=. $GOPATH/src/token_manage/proto/tokens.proto

    This will recreate 2 pb.go files

* Now cd .. to go back to token_manage directory and run client and server codes:

    Run the server code on different terminals with different ports to create server nodes.

        go run server.go -port 50051
        go run server.go -port 50052
        go run server.go -port 50053
        go run server.go -port 50054

    
    On other terminals run client code.

        go run client.go -write -id token1 -name abc -low 0 -mid 10 -high 100 -host localhost -port 50052

        go run client.go -read -id token1 -host localhost -port 50051

    Try running these commands for different tokens on different server nodes. 

## Output:

* The output for all the server nodes are seen in the script and screenshots that I have submitted. 

## Conclusion:

1. I understood the Read-Impose=Write-All protocol.
2. I understood the different between regular and atomic registers.
3. I got to know how to read and use content of YAML file.
4. I learned more about mutexes.
5. I understood the fail-silent and crash stop and I was able to emulate it. 

## References:

1. https://grpc.io/docs/languages/go/quickstart/
2. https://protobuf.dev/getting-started/gotutorial/
3. https://grpc.io/docs/languages/go/basics/
4. https://pkg.go.dev/sync
5. https://gobyexample.com/command-line-flags
6. https://go.dev/tour/basics/7
7. https://yourbasic.org/golang/current-time/
8. https://pkg.go.dev/sync#RWMutex










 
    



 