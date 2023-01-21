package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
)

const AzCLIClientID = "04b07795-8ddb-461a-bbee-02f9e1bf7b46"
const ManagementScope = "https://management.azure.com//.default"
const InteractiveAuthority = "https://login.microsoftonline.com/organizations"

func main() {
	ctx := context.Background()
	cache := &msalCache{
		store: make(map[string][]byte),
	}

	interactiveApp, err := public.New(AzCLIClientID, public.WithAuthority(InteractiveAuthority), public.WithCache(cache))
	if err != nil {
		log.Fatal(err)
	}

	interactiveRes, err := interactiveApp.AcquireTokenInteractive(ctx, []string{ManagementScope})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Interactive Result:")
	fmt.Printf("%#v\n", interactiveRes)

	fmt.Println("Initial Cache")
	fmt.Println("=====")
	cache.Dump()
	fmt.Println("=====")

	defaultCred := &msalPublicAppCredential{
		client:  interactiveApp,
		account: interactiveRes.Account,
	}

	tenantClient, err := armsubscriptions.NewTenantsClient(defaultCred, nil)
	if err != nil {
		log.Fatal(err)
	}

	allTenants := []string{}

	pager := tenantClient.NewListPager(nil)
	for pager.More() {
		res, err := pager.NextPage(ctx)
		if err != nil {
			log.Fatal(err)
		}

		for _, tenantDescription := range res.Value {
			fmt.Printf("*** discovered tenant: %v (%v)\n", *tenantDescription.DisplayName, *tenantDescription.TenantID)
			allTenants = append(allTenants, *tenantDescription.TenantID)
		}
	}

	fmt.Printf("*** discovered %d tenants\n", len(allTenants))

	for _, tenantId := range allTenants {

		fmt.Printf("*** listing subscriptions for tenant: %v\n", tenantId)

		authority := fmt.Sprintf("https://login.microsoftonline.com/%s", tenantId)

		tenantApp, err := public.New(AzCLIClientID, public.WithAuthority(authority), public.WithCache(cache))
		if err != nil {
			log.Fatal(err)
		}

		tenantCred := &msalPublicAppCredential{
			client:  tenantApp,
			account: interactiveRes.Account,
		}

		subscriptionsClient, err := armsubscriptions.NewClient(tenantCred, nil)
		if err != nil {
			log.Panic(err)
		}

		subPager := subscriptionsClient.NewListPager(nil)
		for subPager.More() {
			res, err := subPager.NextPage(ctx)
			if err != nil {
				log.Fatal(err)
			}

			for _, subscription := range res.Value {
				fmt.Printf("*** discovered subscription: %v (%v)\n", *subscription.DisplayName, *subscription.ID)
				allTenants = append(allTenants, *subscription.TenantID)
			}
		}
	}

	fmt.Println("Final Cache")
	fmt.Println("=====")
	cache.Dump()
	fmt.Println("=====")
}

type msalCache struct {
	store map[string][]byte
}

func (c *msalCache) Export(m cache.Marshaler, key string) {
	if v, err := m.Marshal(); err == nil {
		c.store[key] = v
	}
}

func (c *msalCache) Replace(u cache.Unmarshaler, key string) {
	if v, has := c.store[key]; has {
		_ = u.Unmarshal(v)
	}
}

func (c *msalCache) Dump() {
	for k, v := range c.store {
		indented, _ := json.MarshalIndent(json.RawMessage(v), "", " ")
		fmt.Printf("Key: '%s'\n%v\n", k, string(indented))
	}
}

type msalPublicAppCredential struct {
	client  public.Client
	account public.Account
}

func (cred *msalPublicAppCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	res, err := cred.client.AcquireTokenSilent(ctx, options.Scopes, public.WithSilentAccount(cred.account))
	if err != nil {
		return azcore.AccessToken{}, err
	}

	return azcore.AccessToken{
		Token:     res.AccessToken,
		ExpiresOn: res.ExpiresOn,
	}, nil
}
