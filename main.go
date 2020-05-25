package main

import (
	pb "github.com/yeongcheon/pero-chat/gen/go"
	"google.golang.org/grpc"
	"io"
	"log"
	"net"
)

type Plaza struct {
	users []*pb.Plaza_EntryServer
}

func (p *Plaza) broadcast(message *pb.Message) {
	for _, user := range p.users {
		(*user).Send(message)
	}
}

func (p *Plaza) Entry(stream pb.Plaza_EntryServer) error {
	if p.users == nil {
		p.users = make([]*pb.Plaza_EntryServer, 0, 10)
	}
	p.users = append(p.users, &stream)

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

		p.broadcast(message)
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
