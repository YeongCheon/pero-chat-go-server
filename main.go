package main

import (
	"context"
	firebase "firebase.google.com/go"
	pb "github.com/yeongcheon/pero-chat/gen/go"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"io"
	"log"
	"net"
)

var (
	errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")
	errInvalidToken    = status.Errorf(codes.Unauthenticated, "invalid token")
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

func firebaseAuthStreamInterceptor() grpc.StreamServerInterceptor {
	opt := option.WithCredentialsFile("./firebase-adminsdk.json")
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Fatalf("error firebase init app: %v\n", err)
	}

	client, err := app.Auth(context.Background())
	if err != nil {
		log.Fatalf("firebase auth client init err: %v\n", err)
	}

	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if len(md["authorization"]) >= 1 {
				idToken := md["authorization"][0]

				_, err := client.VerifyIDToken(ctx, idToken)
				if err != nil {
					return err
				}
			}

			return handler(srv, ss)
		} else {
			return errInvalidToken
		}
	}
}

func firebaseAuthInterceptor() grpc.UnaryServerInterceptor {
	opt := option.WithCredentialsFile("./firebase-adminsdk.json")
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Fatalf("error firebase init app: %v\n", err)
	}

	client, err := app.Auth(context.Background())
	if err != nil {
		log.Fatalf("firebase auth client init err: %v\n", err)
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			idToken := md["Authorization"][0]
			_, err := client.VerifyIDToken(ctx, idToken)
			if err != nil {
				return nil, err
			}

			return handler(ctx, req)
		} else {
			return nil, errInvalidToken
		}
	}
}

func main() {
	_, _ = credentials.NewServerTLSFromFile("ssl.crt", "ssl.key")
	lis, err := net.Listen("tcp", ":9999")
	if err != nil {
		log.Fatalf("failed to listen : %v", err)
	}

	opts := []grpc.ServerOption{
		// grpc.Creds(creds),
		grpc.StreamInterceptor(firebaseAuthStreamInterceptor()),
		grpc.UnaryInterceptor(firebaseAuthInterceptor()),
	}

	grpcServer := grpc.NewServer(opts...)
	pb.RegisterPlazaServer(grpcServer, &Plaza{})
	if err := grpcServer.Serve(lis); err != nil {
		panic(err)
	}
}
