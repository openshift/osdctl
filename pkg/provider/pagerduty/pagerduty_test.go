package pagerduty

import (
	"fmt"

	pd "github.com/PagerDuty/go-pagerduty"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/brianvoe/gofakeit/v6"
	"go.uber.org/mock/gomock"

	pdMock "github.com/openshift/osdctl/pkg/provider/pagerduty/mocks"
)

func generateIncident() pd.Incident {
	return pd.Incident{
		IncidentNumber: uint(gofakeit.Uint16()),
		Title:          fmt.Sprintf("%s is %s", gofakeit.HackerNoun(), gofakeit.HackeringVerb()),
	}
}

var _ = Describe("Tests the Pagerduty Provider", func() {
	var pdProvider *client
	BeforeEach(func() {
		pdProvider = NewClient()
	})
	Describe("Client Creation", func() {
		Context("WithBaseDomain", func() {
			It("Should correctly populate the base domain", func() {
				pdProvider.WithBaseDomain("foo")
				Expect(pdProvider.baseDomain).To(Equal("foo"))
			})
			It("Should correctly populate the Team ID list", func() {
				pdProvider.WithTeamIdList([]string{"foo", "bar"})
				Expect(pdProvider.teamIds).To(ContainElements("bar", "foo"))
			})
			It("Should correctly populate the userToken", func() {
				pdProvider.WithUserToken("user_token")
				Expect(pdProvider.userToken).To(Equal("user_token"))
			})
			It("Should correctly populate the oauthToken", func() {
				pdProvider.WithOauthToken("oauth_token")
				Expect(pdProvider.oauthToken).To(Equal("oauth_token"))
			})
		})
		Context("Building the Client", func() {
			It("Should build the user_token client when the user client is called", func() {
				err := pdProvider.WithUserToken("token").buildClient()
				Expect(err).To(BeNil())
				Expect(pdProvider.pdclient).To(Not(BeNil()))
			})
			It("Should build the oauth_token client when the client is built", func() {
				err := pdProvider.WithOauthToken("oauth_token").buildClient()
				Expect(err).To(BeNil())
				Expect(pdProvider.pdclient).To(Not(BeNil()))
			})
			It("Should error when neither the user token or oauth token are provided", func() {
				err := pdProvider.buildClient()
				Expect(err).To(Not(BeNil()))
				Expect(pdProvider.pdclient).To(BeNil())
			})
		})
	})

	Describe("Provider Functionality", func() {
		ctrl := gomock.NewController(GinkgoT())

		AfterEach(func() {
			ctrl.Finish()
		})

		Context("GetPDServiceIDs", func() {
			It("Returns an error from the pd client if there's an error with the request", func() {
				m := pdMock.NewMockpdClientInterface(ctrl)
				m.EXPECT().ListServicesWithContext(gomock.Any(), gomock.Any()).Return(&pd.ListServiceResponse{}, fmt.Errorf("Some Error"))
				pdProvider.pdclient = m
				ids, err := pdProvider.GetPDServiceIDs()
				Expect(ids).To(BeEmpty())
				Expect(err).To(Not(BeNil()))
				Expect(err.Error()).To(ContainSubstring("failed to ListServices"))
			})
			It("Correctly parses a list of service ids", func() {
				m := pdMock.NewMockpdClientInterface(ctrl)
				m.EXPECT().ListServicesWithContext(gomock.Any(), gomock.Any()).Return(&pd.ListServiceResponse{Services: []pd.Service{{APIObject: pd.APIObject{ID: "abcd"}}, {APIObject: pd.APIObject{ID: "1234"}}}}, nil)
				pdProvider.pdclient = m
				ids, err := pdProvider.GetPDServiceIDs()
				Expect(err).To(BeNil())
				Expect(ids).To(ContainElements("1234", "abcd"))
			})
		})

		Context("GetFiringAlertsForCluster", func() {
			var emptyIncResponse, singleIncResponse, multipleIncResponse, multiplePageIncResponse *pd.ListIncidentsResponse

			BeforeEach(func() {
				emptyIncResponse = &pd.ListIncidentsResponse{
					Incidents: []pd.Incident{},
				}

				singleIncResponse = &pd.ListIncidentsResponse{
					Incidents: []pd.Incident{
						generateIncident(),
					},
				}

				multipleIncResponse = &pd.ListIncidentsResponse{
					Incidents: []pd.Incident{
						generateIncident(),
						generateIncident(),
						generateIncident(),
					},
				}

				multiplePageIncResponse = &pd.ListIncidentsResponse{
					APIListObject: pd.APIListObject{
						More: true,
					},
					Incidents: []pd.Incident{
						generateIncident(),
						generateIncident(),
						generateIncident(),
						generateIncident(),
						generateIncident(),
					},
				}
			})

			It("Returns an error from the pd client if there's an error with the request", func() {
				m := pdMock.NewMockpdClientInterface(ctrl)
				m.EXPECT().ListIncidentsWithContext(gomock.Any(), gomock.Any()).Return(&pd.ListIncidentsResponse{}, fmt.Errorf("An error"))
				pdProvider.pdclient = m
				incs, err := pdProvider.GetFiringAlertsForCluster([]string{"foo"})
				Expect(incs).To(BeEmpty())
				Expect(err).To(Not(BeNil()))
				Expect(err.Error()).To(ContainSubstring("An error"))
			})

			Context("Handle empty incident lists", func() {
				It("Correctly handles a single Service ID", func() {
					m := pdMock.NewMockpdClientInterface(ctrl)
					m.EXPECT().ListIncidentsWithContext(gomock.Any(), gomock.Any()).Return(emptyIncResponse, nil)
					pdProvider.pdclient = m

					incs, err := pdProvider.GetFiringAlertsForCluster([]string{"foo"})
					Expect(incs["foo"]).To(BeEmpty())
					Expect(err).To(BeNil())
				})
				It("Correctly handles multiple Service IDs", func() {
					m := pdMock.NewMockpdClientInterface(ctrl)
					// This next call happens twice because of the two services that are being described
					m.EXPECT().ListIncidentsWithContext(gomock.Any(), gomock.Any()).Return(emptyIncResponse, nil)
					m.EXPECT().ListIncidentsWithContext(gomock.Any(), gomock.Any()).Return(emptyIncResponse, nil)
					pdProvider.pdclient = m

					incs, err := pdProvider.GetFiringAlertsForCluster([]string{"foo", "bar"})
					Expect(incs["foo"]).To(BeEmpty())
					Expect(incs["bar"]).To(BeEmpty())
					Expect(err).To(BeNil())
				})
			})

			Context("Handle lists with multiple items", func() {
				It("Correctly handles different sized incident response lists", func() {
					m := pdMock.NewMockpdClientInterface(ctrl)
					m.EXPECT().ListIncidentsWithContext(gomock.Any(), gomock.Any()).Return(singleIncResponse, nil)
					m.EXPECT().ListIncidentsWithContext(gomock.Any(), gomock.Any()).Return(multipleIncResponse, nil)
					m.EXPECT().ListIncidentsWithContext(gomock.Any(), gomock.Any()).Return(emptyIncResponse, nil)
					pdProvider.pdclient = m

					incs, err := pdProvider.GetFiringAlertsForCluster([]string{"foo", "bar", "baz"})
					Expect(err).To(BeNil())
					Expect(incs["foo"]).To(HaveLen(1))
					Expect(incs["bar"]).To(HaveLen(3))
					Expect(incs["baz"]).To(BeEmpty())
				})

				It("Correctly handles pagination for a single service in a request with multiple services", func() {
					m := pdMock.NewMockpdClientInterface(ctrl)
					m.EXPECT().ListIncidentsWithContext(gomock.Any(), gomock.Any()).Return(singleIncResponse, nil)
					m.EXPECT().ListIncidentsWithContext(gomock.Any(), gomock.Any()).Return(multiplePageIncResponse, nil)
					m.EXPECT().ListIncidentsWithContext(gomock.Any(), gomock.Any()).Return(multipleIncResponse, nil)
					m.EXPECT().ListIncidentsWithContext(gomock.Any(), gomock.Any()).Return(emptyIncResponse, nil)
					pdProvider.pdclient = m

					incs, err := pdProvider.GetFiringAlertsForCluster([]string{"foo", "bar", "baz"})
					Expect(err).To(BeNil())
					Expect(incs["foo"]).To(HaveLen(1))
					Expect(incs["bar"]).To(HaveLen(8))
					Expect(incs["baz"]).To(BeEmpty())
				})
			})
		})
	})
})
