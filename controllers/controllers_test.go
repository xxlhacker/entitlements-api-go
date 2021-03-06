package controllers

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"errors"
	"testing"

	. "github.com/RedHatInsights/entitlements-api-go/types"
	"github.com/RedHatInsights/platform-go-middlewares/identity"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const DEFAULT_ORG_ID string = "4384938490324"
const DEFAULT_ACCOUNT_NUMBER string = "540155"

func testRequest(method string, path string, accnum string, orgid string, fakeCaller func(string, string) SubscriptionsResponse) (*httptest.ResponseRecorder, map[string]EntitlementsSection, string) {
	req, err := http.NewRequest(method, path, nil)
	Expect(err).To(BeNil(), "NewRequest error was not nil")

	ctx := context.Background()
	ctx = context.WithValue(ctx, identity.Key, identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: accnum,
			Internal: identity.Internal{
				OrgID: orgid,
			},
		},
	})

	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	GetSubscriptions = fakeCaller

	Index()(rr, req)

	out, err := ioutil.ReadAll(rr.Result().Body)
	Expect(err).To(BeNil(), "ioutil.ReadAll error was not nil")

	rr.Result().Body.Close()

	var ret map[string]EntitlementsSection
	json.Unmarshal(out, &ret)

	return rr, ret, string(out)
}

func testRequestWithDefaultOrgId(method string, path string, fakeCaller func(string, string) SubscriptionsResponse) (*httptest.ResponseRecorder, map[string]EntitlementsSection, string) {
	return testRequest(method, path, DEFAULT_ACCOUNT_NUMBER, DEFAULT_ORG_ID, fakeCaller)
}

func fakeGetSubscriptions(expectedOrgID string, expectedSkus string, response SubscriptionsResponse) func(string, string) SubscriptionsResponse {
	return func(orgID string, skus string) SubscriptionsResponse {
		Expect(expectedOrgID).To(Equal(orgID))
		return response
	}
}

func expectPass(res *http.Response) {
	Expect(res.StatusCode).To(Equal(200))
	Expect(res.Header.Get("Content-Type")).To(Equal("application/json"))
}

var _ = Describe("Identity Controller", func() {

	BeforeEach(func() {
		bundleInfo = []Bundle{}
		if err := SetBundleInfo("../test_data/test_bundle.yml"); err != nil {
			panic("Error in test_bundle.yml")
		}
	})

	It("should call GetSubscriptions with the org_id on the context", func() {
		fakeResponse := SubscriptionsResponse{
			StatusCode: 200,
			Data:       []string{"foo", "bar"},
			CacheHit:   false,
		}
		testRequest("GET", "/", DEFAULT_ACCOUNT_NUMBER, "540155", fakeGetSubscriptions("540155", "", fakeResponse))
		testRequest("GET", "/", DEFAULT_ACCOUNT_NUMBER, "deadbeef12", fakeGetSubscriptions("deadbeef12", "", fakeResponse))
	})

	Context("When the Subs API sends back a non-200", func() {
		It("should fail the response", func() {
			rr, _, rawBody := testRequestWithDefaultOrgId("GET", "/", func(string, string) SubscriptionsResponse {
				return SubscriptionsResponse{StatusCode: 503, Data: nil, CacheHit: false}
			})

			var jsonResponse DependencyErrorResponse
			json.Unmarshal([]byte(rawBody), &jsonResponse)

			Expect(rr.Result().StatusCode).To(Equal(500))
			Expect(jsonResponse.Error.DependencyFailure).To(Equal(true))
			Expect(jsonResponse.Error.Service).To(Equal("Subscriptions Service"))
			Expect(jsonResponse.Error.Status).To(Equal(503))
			Expect(jsonResponse.Error.Endpoint).To(Equal("https://subscription.api.redhat.com"))
			Expect(jsonResponse.Error.Message).To(Equal("Got back a non 200 status code from Subscriptions Service"))
		})
	})

	Context("When the Subs API sends back an error", func() {
		It("should fail the response", func() {
			rr, _, rawBody := testRequestWithDefaultOrgId("GET", "/", func(string, string) SubscriptionsResponse {
				return SubscriptionsResponse{StatusCode: 503, Data: nil, CacheHit: false, Error: errors.New("Sub Failure")}
			})

			var jsonResponse DependencyErrorResponse
			json.Unmarshal([]byte(rawBody), &jsonResponse)

			Expect(rr.Result().StatusCode).To(Equal(500))
			Expect(jsonResponse.Error.DependencyFailure).To(Equal(true))
			Expect(jsonResponse.Error.Service).To(Equal("Subscriptions Service"))
			Expect(jsonResponse.Error.Status).To(Equal(503))
			Expect(jsonResponse.Error.Endpoint).To(Equal("https://subscription.api.redhat.com"))
			Expect(jsonResponse.Error.Message).To(Equal("Unexpected error while talking to Subs Service"))
		})
	})

	Context("When the bundles.yml has errors", func() {
		It("should include errors when file is not available", func() {
			bundleInfo = []Bundle{}
			err := SetBundleInfo("no_such_file")
			Expect(len(bundleInfo)).To(Equal(0))
			Expect(err).ToNot(Equal(nil))
		})

		It("should return error for yaml parse errors", func() {
			bundleInfo = []Bundle{}
			err := SetBundleInfo("../test_data/err_bundle.yml")
			Expect(len(bundleInfo)).To(Equal(0))
			Expect(err).ToNot(Equal(nil))
		})
	})

	Context("When the Subs API says we have SKUs to a Bundle", func() {
		It("should give back a valid EntitlementsResponse with that bundle true", func() {
			fakeResponse := SubscriptionsResponse{
				StatusCode: 200,
				Data:       []string{"SVC123", "SVC3851", "MCT3691"},
				CacheHit:   false,
			}

			rr, body, _ := testRequestWithDefaultOrgId("GET", "/", fakeGetSubscriptions(DEFAULT_ORG_ID, "SVC3124,MCT3691", fakeResponse))
			expectPass(rr.Result())
			Expect(body["TestBundle1"].IsEntitled).To(Equal(true))
		})
	})

	Context("When the Subs API says we *dont* have SKUs to a Bundle", func() {
		It("should give back a valid EntitlementsResponse with that bundle true", func() {
			fakeResponse := SubscriptionsResponse{
				StatusCode: 200,
				Data:       []string{"SVC123", "SVC3851", "MCT3691"},
				CacheHit:   false,
			}

			rr, body, _ := testRequestWithDefaultOrgId("GET", "/", fakeGetSubscriptions(DEFAULT_ORG_ID, "SVC3124,MCT3691", fakeResponse))
			expectPass(rr.Result())
			Expect(body["TestBundle1"].IsEntitled).To(Equal(true))
			Expect(body["TestBundle2"].IsEntitled).To(Equal(false))
		})
	})

	Context("When the account number is -1 or '' ", func() {
		fakeResponse := SubscriptionsResponse{
			StatusCode: 200,
			Data:       []string{"SVC123", "MCT1122", "SVC7788"},
			CacheHit:   false,
		}

		It("should give back a valid EntitlementsResponse with bundles using Valid Account Number false", func() {
			// testing with account number "-1"
			rr, body, _ := testRequest("GET", "/", "-1", DEFAULT_ORG_ID, fakeGetSubscriptions(DEFAULT_ORG_ID, "test", fakeResponse))
			expectPass(rr.Result())
			Expect(body["TestBundle1"].IsEntitled).To(Equal(false))
			Expect(body["TestBundle2"].IsEntitled).To(Equal(false))
			Expect(body["TestBundle3"].IsEntitled).To(Equal(true))
			Expect(body["TestBundle4"].IsEntitled).To(Equal(false))
		})

		It("should give back a valid EntitlementsResponse with bundles using Valid Account Number false", func() {
			// testing with account number ""
			rr, body, _ := testRequest("GET", "/", "", DEFAULT_ORG_ID, fakeGetSubscriptions(DEFAULT_ORG_ID, "test", fakeResponse))
			expectPass(rr.Result())
			Expect(body["TestBundle1"].IsEntitled).To(Equal(false))
			Expect(body["TestBundle2"].IsEntitled).To(Equal(false))
			Expect(body["TestBundle3"].IsEntitled).To(Equal(true))
			Expect(body["TestBundle4"].IsEntitled).To(Equal(false))
		})
	})

	Context("When a bundle uses only Valid Account Number", func() {
		fakeResponse := SubscriptionsResponse{
			StatusCode: 200,
			Data:       []string{},
			CacheHit:   false,
		}

		It("should give back a valid EntitlementsResponse with that bundle true", func() {
			// testing with account number ""
			rr, body, _ := testRequest("GET", "/", "123456", DEFAULT_ORG_ID, fakeGetSubscriptions(DEFAULT_ORG_ID, "test", fakeResponse))
			expectPass(rr.Result())
			Expect(body["TestBundle1"].IsEntitled).To(Equal(false))
			Expect(body["TestBundle2"].IsEntitled).To(Equal(false))
			Expect(body["TestBundle3"].IsEntitled).To(Equal(true))
			Expect(body["TestBundle4"].IsEntitled).To(Equal(true))
		})

	})

	Context("When the Subs API says we have SKUs to a Bundle", func() {
		It("should set IsTrial to `true` when the only SKU(s) entitled are trials", func() {
			fakeResponse := SubscriptionsResponse{
				StatusCode: 200,
				Data:       []string{"SVC123", "SVC3851", "MCT1122"},
				CacheHit:   false,
			}

			rr, body, _ := testRequestWithDefaultOrgId("GET", "/", fakeGetSubscriptions(DEFAULT_ORG_ID, "", fakeResponse))
			expectPass(rr.Result())
			Expect(body["TestBundle1"].IsTrial).To(Equal(false))
			Expect(body["TestBundle2"].IsTrial).To(Equal(true))
		})

		It("should set IsTrial to `false` when one ore more SKU(s) entitled are not trials", func() {
			fakeResponse := SubscriptionsResponse{
				StatusCode: 200,
				Data:       []string{"SVC123", "SVC3851", "MCT1122", "SVC3344"},
				CacheHit:   false,
			}

			rr, body, _ := testRequestWithDefaultOrgId("GET", "/", fakeGetSubscriptions(DEFAULT_ORG_ID, "", fakeResponse))
			expectPass(rr.Result())
			Expect(body["TestBundle1"].IsTrial).To(Equal(false))
			Expect(body["TestBundle2"].IsTrial).To(Equal(false))
		})

		It("should set IsTrial to `false` when the user is not entitled because they have no SKUs", func() {
			fakeResponse := SubscriptionsResponse{
				StatusCode: 200,
				Data:       []string{},
				CacheHit:   false,
			}

			rr, body, _ := testRequestWithDefaultOrgId("GET", "/", fakeGetSubscriptions(DEFAULT_ORG_ID, "", fakeResponse))
			expectPass(rr.Result())
			Expect(body["TestBundle1"].IsTrial).To(Equal(false))
			Expect(body["TestBundle2"].IsTrial).To(Equal(false))
			Expect(body["TestBundle1"].IsEntitled).To(Equal(false))
			Expect(body["TestBundle2"].IsEntitled).To(Equal(false))
		})
	})
})

func BenchmarkRequest(b *testing.B) {
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		fakeResponse := SubscriptionsResponse{
			StatusCode: 200,
			Data:       []string{"SVC123", "SVC3851", "MCT3691"},
			CacheHit:   false,
		}

		testRequestWithDefaultOrgId("GET", "/", fakeGetSubscriptions(DEFAULT_ORG_ID, "SVC3124,MCT3691", fakeResponse))
	}
}
