package simples3

import (
	"os"
	"testing"
)

func TestS3_FileUpload(t *testing.T) {
	testTxt, err := os.Open("test.txt")
	if err != nil {
		return
	}
	defer testTxt.Close()

	type fields struct {
		AccessKey string
		SecretKey string
		Region    string
	}
	type args struct {
		u UploadInput
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "Upload test.txt",
			fields: fields{
				AccessKey: os.Getenv("AWS_S3_ACCESS_KEY"),
				SecretKey: os.Getenv("AWS_S3_SECRET_KEY"),
				Region:    os.Getenv("AWS_S3_REGION"),
			},
			args: args{
				UploadInput{
					Bucket:      "zerodha-testbucket",
					ObjectKey:   "test.txt",
					ContentType: "text/plain",
					FileName:    "test.txt",
					Body:        testTxt,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s3 := New(tt.fields.Region, tt.fields.AccessKey, tt.fields.SecretKey)
			if err := s3.FileUpload(tt.args.u); (err != nil) != tt.wantErr {
				t.Errorf("S3.FileUpload() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestS3_FileDelete(t *testing.T) {
	type fields struct {
		AccessKey string
		SecretKey string
		Region    string
	}
	type args struct {
		u DeleteInput
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "Delete test.txt",
			fields: fields{
				AccessKey: os.Getenv("MONGO_PASS"),
				SecretKey: os.Getenv("AWS_S3_ACCESS_KEY"),
				Region:    os.Getenv("AWS_S3_ACCESS_KEY"),
			},
			args: args{
				DeleteInput{
					Bucket:    "zerodha-testbucket",
					ObjectKey: "test.txt",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s3 := New(tt.fields.Region, tt.fields.AccessKey, tt.fields.SecretKey)
			if err := s3.FileDelete(tt.args.u); (err != nil) != tt.wantErr {
				t.Errorf("S3.FileDelete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
