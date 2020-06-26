package aws

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/golang/mock/gomock"
	"github.com/openshift/osd-utils-cli/pkg/provider/aws/mock"
)

func TestDeleteS3BucketsWithPrefix(t *testing.T) {
	g := NewGomegaWithT(t)
	testCases := []struct {
		title        string
		setupAWSMock func(r *mock.MockClientMockRecorder)
		prefix       string
		errExpected  bool
	}{
		{
			title: "List buckets return error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.ListBuckets(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1)
			},
			prefix:      "",
			errExpected: true,
		},
		{
			title: "List buckets return empty buckets",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.ListBuckets(gomock.Any()).Return(&s3.ListBucketsOutput{Buckets: []*s3.Bucket{}}, nil).Times(1)
			},
			prefix:      "",
			errExpected: false,
		},
		{
			title: "List buckets return buckets but don't match prefix",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				r.ListBuckets(gomock.Any()).Return(
					&s3.ListBucketsOutput{
						Buckets: []*s3.Bucket{{Name: aws.String("foo")}},
					}, nil).Times(1)
			},
			prefix:      "bar",
			errExpected: false,
		},
		{
			title: "List objects return error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListBuckets(gomock.Any()).Return(
						&s3.ListBucketsOutput{
							Buckets: []*s3.Bucket{{Name: aws.String("foo")}},
						}, nil).Times(1),
					r.ListObjects(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			prefix:      "foo",
			errExpected: true,
		},
		{
			title: "delete object return error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListBuckets(gomock.Any()).Return(
						&s3.ListBucketsOutput{
							Buckets: []*s3.Bucket{{Name: aws.String("foo")}},
						}, nil).Times(1),
					r.ListObjects(gomock.Any()).Return(&s3.ListObjectsOutput{
						Name: aws.String("aws"),
						Contents: []*s3.Object{
							{
								Key: aws.String("foo"),
							},
						},
					}, nil).Times(1),
					r.DeleteObjects(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			prefix:      "foo",
			errExpected: true,
		},
		{
			title: "delete bucket returns error",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListBuckets(gomock.Any()).Return(
						&s3.ListBucketsOutput{
							Buckets: []*s3.Bucket{{Name: aws.String("foo")}},
						}, nil).Times(1),
					r.ListObjects(gomock.Any()).Return(&s3.ListObjectsOutput{
						Name: aws.String("aws"),
						Contents: []*s3.Object{
							{
								Key: aws.String("foo"),
							},
						},
					}, nil).Times(1),
					r.DeleteObjects(gomock.Any()).Return(nil, nil).Times(1),
					r.DeleteBucket(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			prefix:      "foo",
			errExpected: true,
		},
		{
			title: "delete bucket success",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListBuckets(gomock.Any()).Return(
						&s3.ListBucketsOutput{
							Buckets: []*s3.Bucket{{Name: aws.String("foo")}},
						}, nil).Times(1),
					r.ListObjects(gomock.Any()).Return(&s3.ListObjectsOutput{
						Name: aws.String("aws"),
						Contents: []*s3.Object{
							{
								Key: aws.String("foo"),
							},
						},
					}, nil).Times(1),
					r.DeleteObjects(gomock.Any()).Return(nil, nil).Times(1),
					r.DeleteBucket(gomock.Any()).Return(nil, nil).Times(1),
				)
			},
			prefix:      "foo",
			errExpected: false,
		},
		{
			title: "list 2nd bucket objects failed",
			setupAWSMock: func(r *mock.MockClientMockRecorder) {
				gomock.InOrder(
					r.ListBuckets(gomock.Any()).Return(
						&s3.ListBucketsOutput{
							Buckets: []*s3.Bucket{
								{Name: aws.String("foo1")},
								{Name: aws.String("foo2")},
							},
						}, nil).Times(1),
					r.ListObjects(gomock.Any()).Return(&s3.ListObjectsOutput{
						Name: aws.String("aws"),
						Contents: []*s3.Object{
							{
								Key: aws.String("foo"),
							},
						},
					}, nil).Times(1),
					r.DeleteObjects(gomock.Any()).Return(nil, nil).Times(1),
					r.DeleteBucket(gomock.Any()).Return(nil, nil).Times(1),
					r.ListObjects(gomock.Any()).Return(nil, errors.New("FakeError")).Times(1),
				)
			},
			prefix:      "foo",
			errExpected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			mocks := setupDefaultMocks(t)
			tc.setupAWSMock(mocks.mockAWSClient.EXPECT())

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.
			// after mocks is defined
			defer mocks.mockCtrl.Finish()

			err := DeleteS3BucketsWithPrefix(mocks.mockAWSClient, tc.prefix)
			if tc.errExpected {
				g.Expect(err).Should(HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(HaveOccurred())
			}
		})
	}
}
