package awss3

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/bishtawi/migrate/v4/source"
)

func init() {
	source.Register("s3", &s3Driver{})
}

type s3Driver struct {
	s3client   s3iface.S3API
	config     *Config
	migrations *source.Migrations
}

type Config struct {
	Bucket string
	Prefix string
}

func (s *s3Driver) Open(folder string) (source.Driver, error) {
	driver, err := newS3Driver(folder)
	if err != nil {
		return nil, err
	}

	if err := driver.loadMigrations(); err != nil {
		return nil, err
	}

	return driver, nil
}

func WithInstance(s3client s3iface.S3API, config *Config) (source.Driver, error) {
	driver := &s3Driver{
		config:     config,
		s3client:   s3client,
		migrations: source.NewMigrations(),
	}

	if err := driver.loadMigrations(); err != nil {
		return nil, err
	}

	return driver, nil
}

func newS3Driver(folder string) (*s3Driver, error) {
	u, err := url.Parse(folder)
	if err != nil {
		return nil, err
	}

	sess, err := session.NewSession()
	if err != nil {
		return nil, err
	}

	prefix := strings.Trim(u.Path, "/")
	if prefix != "" {
		prefix += "/"
	}

	return &s3Driver{
		config: &Config{
			Bucket: u.Host,
			Prefix: prefix,
		},
		s3client:   s3.New(sess),
		migrations: source.NewMigrations(),
	}, nil
}

func (s *s3Driver) loadMigrations() error {
	output, err := s.s3client.ListObjects(&s3.ListObjectsInput{
		Bucket:    aws.String(s.config.Bucket),
		Prefix:    aws.String(s.config.Prefix),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		return err
	}
	for _, object := range output.Contents {
		_, fileName := path.Split(aws.StringValue(object.Key))
		m, err := source.DefaultParse(fileName)
		if err != nil {
			continue
		}
		if !s.migrations.Append(m) {
			return fmt.Errorf("unable to parse file %v", aws.StringValue(object.Key))
		}
	}
	return nil
}

func (s *s3Driver) Close() error {
	return nil
}

func (s *s3Driver) First() (uint, error) {
	v, ok := s.migrations.First()
	if !ok {
		return 0, os.ErrNotExist
	}
	return v, nil
}

func (s *s3Driver) Prev(version uint) (uint, error) {
	v, ok := s.migrations.Prev(version)
	if !ok {
		return 0, os.ErrNotExist
	}
	return v, nil
}

func (s *s3Driver) Next(version uint) (uint, error) {
	v, ok := s.migrations.Next(version)
	if !ok {
		return 0, os.ErrNotExist
	}
	return v, nil
}

func (s *s3Driver) ReadUp(version uint) (io.ReadCloser, string, error) {
	if m, ok := s.migrations.Up(version); ok {
		return s.open(m)
	}
	return nil, "", os.ErrNotExist
}

func (s *s3Driver) ReadDown(version uint) (io.ReadCloser, string, error) {
	if m, ok := s.migrations.Down(version); ok {
		return s.open(m)
	}
	return nil, "", os.ErrNotExist
}

func (s *s3Driver) open(m *source.Migration) (io.ReadCloser, string, error) {
	key := path.Join(s.config.Prefix, m.Raw)
	object, err := s.s3client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.config.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", err
	}
	return object.Body, m.Identifier, nil
}
