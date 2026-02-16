package db 

import (
	"context"
	"log"
	"os"

	firebase "firebase.google.com/go"
	"cloud.google.com/go/firestore"
	"google.golang.org/api/option"
)

type FirestoreClient struct {
	Client *firestore.Client
}


func NewFirestoreClient(ctx context.Context, credentialsPath string) *FirestoreClient {
		opt:= option.WithCredentialsFile(credentialsPath)
		
		app, err := firebase.NewApp(ctx, nil, opt)
		if err != nil {
			log.Fatalf("error initializing app: %v\n", err)
		}
		
		client, err := app.Firestore(ctx)
		if err != nil {
			log.Fatalf("error creating firestore client: %v\n", err)
		}
		
		return &FirestoreClient{Client: client}
	
}