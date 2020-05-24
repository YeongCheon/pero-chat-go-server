package main

import (
	pb "github.com/yeongcheon/pero-chat/gen/go"
	"google.golang.org/grpc"
	"io"
	"log"
	"net"
)

type Plaza struct{}

func (p *Plaza) Entry(stream pb.Plaza_EntryServer) error {
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := in.Name
		content := in.Content

		message := &pb.Message{
			Name:    name,
			Content: content,
		}

		stream.Send(message)
	}
}

func main() {
	lis, err := net.Listen("tcp", ":9999")
	if err != nil {
		log.Fatalf("failed to listen : %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterPlazaServer(grpcServer, &Plaza{})
	grpcServer.Serve(lis)
}
