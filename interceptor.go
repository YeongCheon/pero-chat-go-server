package main

import (
	"context"
	firebase "firebase.google.com/go"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"log"
)

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

				authToken, err := client.VerifyIDToken(ctx, idToken)
				if err != nil {
					return err
				}

				ctx = context.WithValue(ctx, "uid", authToken.UID)
			}
		} else {
			return errInvalidToken
		}

		return handler(srv, newWrappedStream(ctx, ss))
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
			authorization := md["authorization"]
			if len(authorization) == 0 {
				return nil, errInvalidToken
			}

			idToken := authorization[0]

			authToken, err := client.VerifyIDToken(ctx, idToken)
			if err != nil {
				return nil, err
			}

			ctx = context.WithValue(ctx, "uid", authToken.UID)
			return handler(ctx, req)
		} else {
			return nil, errInvalidToken
		}
	}
}

func unaryLoggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	log.Printf("%s", req)
	return handler(ctx, req)
}
