package servicelog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/internal/servicelog"
	"github.com/stretchr/testify/assert"
)

func TestSetup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Setup Suite")
}

var _ = Describe("Test posting service logs", func() {
	var options *PostCmdOptions

	BeforeEach(func() {
		options = &PostCmdOptions{
			Overrides: []string{
				"description=new description",
				"summary=new summary",
			},
			Message: servicelog.Message{
				Summary:      "The original summary",
				InternalOnly: false,
			},
		}
	})

	Context("overriding a field", func() {
		It("overrides string fields successfully", func() {
			overrideString := "Overridden Summary"
			err := options.overrideField("summary", overrideString)

			Expect(err).ShouldNot(HaveOccurred())
			Expect(options.Message.Summary).To(Equal(overrideString))
		})

		It("overrides bool fields correctly", func() {
			Expect(options.Message.InternalOnly).ToNot(Equal(true))

			err := options.overrideField("internal_only", "true")

			Expect(err).ShouldNot(HaveOccurred())
			Expect(options.Message.InternalOnly).To(Equal(true))
		})

		It("errors when overriding a field that does not exist", func() {
			err := options.overrideField("does_not_exist", "")

			Expect(err).Should(HaveOccurred())
		})

		It("errors when overriding a bool with an unparsable string", func() {
			err := options.overrideField("internal_only", "ThisIsNotABool")

			Expect(err).Should(HaveOccurred())
		})

		It("errors when overriding an unsupported data type", func() {
			err := options.overrideField("doc_references", "DoesntMatter")

			Expect(err).Should(HaveOccurred())
		})
	})

	Context("parsing overrides", func() {
		It("parses correctly", func() {
			overrideMap, err := options.parseOverrides()

			Expect(err).ShouldNot(HaveOccurred())
			Expect(overrideMap).To(HaveKey("description"))
			Expect(overrideMap["description"]).To(Equal("new description"))
			Expect(overrideMap).To(HaveKey("summary"))
			Expect(overrideMap["summary"]).To(Equal("new summary"))
		})

		It("fails when an option contains no equals sign", func() {
			options.Overrides = []string{
				"THISDOESNOTHAVEANEQUALS",
			}

			_, err := options.parseOverrides()

			Expect(err).Should(HaveOccurred())
		})

		It("fails when an option has no key", func() {
			options.Overrides = []string{
				"=VALUE",
			}

			_, err := options.parseOverrides()

			Expect(err).Should(HaveOccurred())
		})

		It("fails when an option has no value", func() {
			options.Overrides = []string{
				"KEY=",
			}

			_, err := options.parseOverrides()

			Expect(err).Should(HaveOccurred())
		})
	})

	Context("initializing PostCmdOptions", func() {
		It("initializes the PostCmdOptions structure", func() {
			options := &PostCmdOptions{}
			err := options.Init()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(options.successfulClusters).NotTo(BeNil())
			Expect(options.failedClusters).NotTo(BeNil())
		})
	})

	Context("validating PostCmdOptions", func() {
		It("fails when cluster-id is missing and no filter is provided", func() {
			options := &PostCmdOptions{}
			err := options.Validate()
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(Equal("no cluster identifier has been found"))
		})

		It("validates successfully with a cluster-id", func() {
			options := &PostCmdOptions{
				ClusterId: "test-cluster",
			}
			err := options.Validate()
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("validates successfully with a filter", func() {
			options := &PostCmdOptions{
				filterParams: []string{"cloud_provider.id is 'gcp'"},
			}
			err := options.Validate()
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	Context("newPostCmd function", func() {
		It("creates a new post command", func() {
			cmd := newPostCmd()
			Expect(cmd).NotTo(BeNil())
			Expect(cmd.Use).To(Equal("post CLUSTER_ID"))
			Expect(cmd.Short).To(Equal("Post a service log to a cluster or list of clusters"))
		})
	})

	Context("getDocClusterType function", func() {
		It("returns the correct cluster type from the documentation URL", func() {
			message := "Check the documentation at https://docs.openshift.com/dedicated/welcome/index.html"
			clusterType := getDocClusterType(message)
			Expect(clusterType).To(Equal("osd"))
		})

		It("returns an empty string when the documentation URL is not present", func() {
			message := "No documentation URL here."
			clusterType := getDocClusterType(message)
			Expect(clusterType).To(Equal(""))
		})
	})

	Context("accessing files", func() {
		It("reads a local file successfully", func() {
			content := []byte("test content")
			tmpfile, err := os.CreateTemp("", "example")
			Expect(err).ShouldNot(HaveOccurred())

			defer os.Remove(tmpfile.Name())

			_, err = tmpfile.Write(content)
			Expect(err).ShouldNot(HaveOccurred())

			err = tmpfile.Close()
			Expect(err).ShouldNot(HaveOccurred())

			fileContent, err := options.accessFile(tmpfile.Name())
			Expect(err).ShouldNot(HaveOccurred())
			Expect(fileContent).To(Equal(content))
		})

		It("reads a URL successfully", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("test content"))
			}))
			defer server.Close()

			fileContent, err := options.accessFile(server.URL)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(fileContent).To(Equal([]byte("test content")))
		})

		It("returns an error for a non-existent file", func() {
			_, err := options.accessFile("non-existent-file")
			Expect(err).Should(HaveOccurred())
		})

		It("returns an error for a directory", func() {
			dir, err := os.MkdirTemp("", "exampledir")
			Expect(err).ShouldNot(HaveOccurred())

			defer os.RemoveAll(dir)

			_, err = options.accessFile(dir)
			Expect(err).Should(HaveOccurred())
		})
	})

	Context("parsing template", func() {
		It("parses a valid JSON template successfully", func() {
			template := servicelog.Message{
				Summary:      "Test Summary",
				Description:  "Test Description",
				InternalOnly: true,
			}
			jsonData, err := json.Marshal(template)
			Expect(err).ShouldNot(HaveOccurred())

			err = options.parseTemplate(jsonData)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(options.Message.Summary).To(Equal(template.Summary))
			Expect(options.Message.Description).To(Equal(template.Description))
			Expect(options.Message.InternalOnly).To(Equal(template.InternalOnly))
		})

		It("returns an error for an invalid JSON template", func() {
			jsonData := []byte(`{"summary": "Test Summary", "description": "Test Description", "internal_only": "this should be bool"}`)

			err := options.parseTemplate(jsonData)
			Expect(err).Should(HaveOccurred())
		})
	})

	Context("cleaning up", func() {
		It("cleans up successfully", func() {
			successful := map[string]string{
				"cluster-1": "success",
			}

			failed := map[string]string{}

			clusters := []*v1.Cluster{}
			cluster1, _ := v1.NewCluster().ExternalID("cluster-1").Build()
			cluster2, _ := v1.NewCluster().ExternalID("cluster-2").Build()

			clusters = append(clusters, cluster1)
			clusters = append(clusters, cluster2)

			options := &PostCmdOptions{
				successfulClusters: successful,
				failedClusters:     failed,
			}

			options.cleanUp(clusters)

			// cluster-1 should not be in failedClusters
			Expect(options.failedClusters).ToNot(HaveKey("cluster-1"))

			// cluster-2 should be in failedClusters
			Expect(options.failedClusters).To(HaveKey("cluster-2"))
			Expect(options.failedClusters["cluster-2"]).To(Equal("cannot send message due to program interruption"))
		})
	})
})

func TestParseUserParameters(t *testing.T) {
	tests := []struct {
		name         string
		input        []string
		expectNames  []string
		expectValues []string
	}{
		{
			name:         "Valid Parameters",
			input:        []string{"FOO=BAR", "BAZ=QUX"},
			expectNames:  []string{"${FOO}", "${BAZ}"},
			expectValues: []string{"BAR", "QUX"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := &PostCmdOptions{TemplateParams: tt.input}
			userParameterNames = []string{}
			userParameterValues = []string{}

			options.parseUserParameters()

			assert.Equal(t, tt.expectNames, userParameterNames)
			assert.Equal(t, tt.expectValues, userParameterValues)
		})
	}
}
func TestPrintTemplate(t *testing.T) {
	tests := []struct {
		name     string
		input    *PostCmdOptions
		expected error
	}{
		{
			name: "Valid input with no errors",
			input: &PostCmdOptions{
				Message: servicelog.Message{
					Severity:      "info",
					ServiceName:   "TestService",
					ClusterID:     "cluster-123",
					Summary:       "Test Summary",
					Description:   "This is a test message",
					InternalOnly:  false,
					EventStreamID: "event-456",
					DocReferences: []string{"doc-1", "doc-2"},
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.printTemplate()
			assert.Equal(t, tt.expected, err)
		})
	}
}
func TestPrintClusters(t *testing.T) {
	tests := []struct {
		name     string
		input    []*v1.Cluster
		expected error
	}{
		{
			name: "Valid clusters input with no errors",
			input: func() []*v1.Cluster {
				clusters := []*v1.Cluster{}
				cluster1, _ := v1.NewCluster().ExternalID("cluster-1").Build()
				cluster2, _ := v1.NewCluster().ExternalID("cluster-2").Build()
				clusters = append(clusters, cluster1)
				clusters = append(clusters, cluster2)
				return clusters
			}(),
			expected: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &PostCmdOptions{}
			err := o.printClusters(tt.input)
			assert.Equal(t, tt.expected, err)
		})
	}
}
func TestListMessagedClusters(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected error
	}{
		{
			name:     "valid_clusters_input_with_no_errors",
			input:    nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusters := make(map[string]string, 4)
			clusters["Id1"] = "Status1"
			clusters["Id2"] = "Status2"
			clusters["Id3"] = "Status3"
			clusters["Id4"] = "Status4"
			o := &PostCmdOptions{}
			err := o.listMessagedClusters(clusters)
			assert.Equal(t, tt.expected, err)
		})
	}
}
func TestParseClustersFile(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected error
	}{
		{
			name:     "Valid JSON input with no errors",
			input:    []byte(`{"clusters":["cluster-1","cluster-2"]}`),
			expected: nil,
		},
		{
			name:     "Invalid JSON input with errors",
			input:    []byte(`{"clusters":["cluster-1","cluster-2"`),
			expected: assert.AnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := &PostCmdOptions{}
			err := options.parseClustersFile(tt.input)
			if tt.expected == nil {
				assert.NoError(t, err)
				assert.Equal(t, 2, len(options.ClustersFile.Clusters))
				assert.Equal(t, "cluster-1", options.ClustersFile.Clusters[0])
				assert.Equal(t, "cluster-2", options.ClustersFile.Clusters[1])
			} else {
				assert.Error(t, err)
			}
		})
	}
}
func TestReplaceFlags(t *testing.T) {
	tests := []struct {
		name           string
		inputOptions   PostCmdOptions
		flagName       string
		flagValue      string
		expectedMsg    string
		expectedFilter string
	}{
		{
			name: "Valid Flag Replacement in Message",
			inputOptions: PostCmdOptions{
				Message: servicelog.Message{Summary: "This is a FILTERREPLACE test"},
			},
			flagName:    "FILTERREPLACE",
			flagValue:   "successful",
			expectedMsg: "This is a successful test",
		},
		{
			name: "Valid Flag Replacement in filtersFromFile",
			inputOptions: PostCmdOptions{
				filtersFromFile: "Some filter with FILTERREPLACE",
			},
			flagName:       "FILTERREPLACE",
			flagValue:      "value",
			expectedFilter: "Some filter with value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.inputOptions.replaceFlags(tt.flagName, tt.flagValue)
			assert.Equal(t, tt.expectedMsg, tt.inputOptions.Message.Summary)
			assert.Equal(t, tt.expectedFilter, tt.inputOptions.filtersFromFile)

		})
	}
}
func TestCheckLeftovers(t *testing.T) {
	tests := []struct {
		name         string
		inputOptions PostCmdOptions
		excludes     []string
	}{
		{
			name: "No Leftovers",
			inputOptions: PostCmdOptions{
				Message: servicelog.Message{Summary: "This is a test"},
			},
			excludes: []string{},
		},
		{
			name: "Leftovers Found in Message",
			inputOptions: PostCmdOptions{
				Message: servicelog.Message{Summary: "This is a ${PLACEHOLDER} test"},
			},
			excludes: []string{"${PLACEHOLDER}"},
		},
		{
			name: "Leftovers Found in filtersFromFile",
			inputOptions: PostCmdOptions{
				filtersFromFile: "Some filter with ${FILTER}",
			},
			excludes: []string{"${FILTER}"},
		},
		{
			name: "Excluded Leftovers",
			inputOptions: PostCmdOptions{
				Message: servicelog.Message{Summary: "This is a ${PLACEHOLDER} test"},
			},
			excludes: []string{"${PLACEHOLDER}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.inputOptions.checkLeftovers(tt.excludes)
			// no assert is required as checkleftover doesn't return anything, We just have to ensure no Fatal logs are logged.
		})
	}
}
func TestReadTemplate(t *testing.T) {
	tests := []struct {
		name        string
		options     PostCmdOptions
		expectedMsg servicelog.Message
		prepare     func()
	}{
		{
			name: "Internal only template",
			options: PostCmdOptions{
				InternalOnly: true,
			},
			expectedMsg: servicelog.Message{
				Severity:     "Info",
				ServiceName:  "SREManualAction",
				Summary:      "INTERNAL ONLY, DO NOT SHARE WITH CUSTOMER",
				Description:  "${MESSAGE}",
				InternalOnly: true,
			},
		},
		{
			name: "Pre-canned template with overrides",
			options: PostCmdOptions{
				InternalOnly: false,
				Overrides:    []string{"some_override"},
			},
			expectedMsg: servicelog.Message{
				Severity:     "Info",
				ServiceName:  "SREManualAction",
				InternalOnly: true,
			},
		},
		{
			name: "Template file provided",
			options: PostCmdOptions{
				InternalOnly: false,
				Template:     "template.json",
			},
			expectedMsg: servicelog.Message{
				Severity:     "Info",
				ServiceName:  "TestService",
				Summary:      "Test Summary",
				Description:  "Test Description",
				InternalOnly: true,
			},
			prepare: func() {
				fileContent := `{
					"severity": "Info",
					"service_name": "TestService",
					"summary": "Test Summary",
					"description": "Test Description",
					"internal_only": true
				}`
				if err := os.WriteFile("template.json", []byte(fileContent), 0644); err != nil {
					t.Fatalf("Failed to create template file: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.prepare != nil {
				tt.prepare()
			}

			// Run the function
			tt.options.readTemplate()

			// Check if the expected message is set correctly
			assert.Equal(t, tt.expectedMsg, tt.options.Message)

			// Remove the template file if it was used
			if tt.options.Template == "template.json" {
				os.Remove(tt.options.Template)
			}
		})
	}
}
func TestReadFilterFile(t *testing.T) {
	tests := []struct {
		name           string
		options        PostCmdOptions
		expectedFilter string
		prepare        func(t *testing.T, options *PostCmdOptions)
	}{
		{
			name: "No filter files specified",
			options: PostCmdOptions{
				filterFiles: []string{},
			},
			expectedFilter: "",
		},
		{
			name: "One filter file",
			options: PostCmdOptions{
				filterFiles: []string{},
			},
			expectedFilter: "(Filter content from filter1)",
			prepare: func(t *testing.T, options *PostCmdOptions) {
				// Create a temporary filter file
				fileContent := "Filter content from filter1"
				tmpFile, err := os.CreateTemp("", "filter1_*.txt")
				if err != nil {
					t.Fatalf("Failed to create temp filter file: %v", err)
				}
				defer tmpFile.Close()
				if _, err := tmpFile.Write([]byte(fileContent)); err != nil {
					t.Fatalf("Failed to write to temp filter file: %v", err)
				}
				options.filterFiles = append(options.filterFiles, tmpFile.Name())
			},
		},
		{
			name: "Multiple filter files",
			options: PostCmdOptions{
				filterFiles: []string{},
			},
			expectedFilter: "(Filter content from filter1) and (Filter content from filter2)",
			prepare: func(t *testing.T, options *PostCmdOptions) {
				fileContent1 := "Filter content from filter1"
				fileContent2 := "Filter content from filter2"
				tmpFile1, err := os.CreateTemp("", "filter1_*.txt")
				if err != nil {
					t.Fatalf("Failed to create temp filter1 file: %v", err)
				}
				defer tmpFile1.Close()
				tmpFile2, err := os.CreateTemp("", "filter2_*.txt")
				if err != nil {
					t.Fatalf("Failed to create temp filter2 file: %v", err)
				}
				defer tmpFile2.Close()
				if _, err := tmpFile1.Write([]byte(fileContent1)); err != nil {
					t.Fatalf("Failed to write to temp filter1 file: %v", err)
				}
				if _, err := tmpFile2.Write([]byte(fileContent2)); err != nil {
					t.Fatalf("Failed to write to temp filter2 file: %v", err)
				}
				options.filterFiles = []string{tmpFile1.Name(), tmpFile2.Name()}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.prepare != nil {
				tt.prepare(t, &tt.options)
			}
			tt.options.readFilterFile()
			assert.Equal(t, tt.expectedFilter, tt.options.filtersFromFile)
			for _, filterFile := range tt.options.filterFiles {
				os.Remove(filterFile)
			}
		})
	}
}

// Can't Handle positive case as it is going to logs.Fatal().
func TestPrintPostOutput(t *testing.T) {
	tests := []struct {
		name         string
		inputOptions PostCmdOptions
	}{
		{
			name: "No clusters",
			inputOptions: PostCmdOptions{
				successfulClusters: map[string]string{},
				failedClusters:     map[string]string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.inputOptions.printPostOutput()
		})
	}
}
