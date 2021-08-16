/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package identityprovider

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/eks/eksiface"
	"github.com/go-logr/logr"

	"sigs.k8s.io/cluster-api-provider-aws/pkg/planner"
)

// NewPlan creates plan to manage EKS OIDC identity provider association.
func NewPlan(clusterName string, currentIdentityProvider, desiredIdentityProvider *OidcIdentityProviderConfig, client eksiface.EKSAPI, log logr.Logger) planner.Plan {
	return &plan{
		currentIdentityProvider: currentIdentityProvider,
		desiredIdentityProvider: desiredIdentityProvider,
		eksClient:               client,
		clusterName:             clusterName,
		log:                     log,
	}
}

// Plan is a plan that will manage EKS OIDC identity provider association.
type plan struct {
	currentIdentityProvider *OidcIdentityProviderConfig
	desiredIdentityProvider *OidcIdentityProviderConfig
	eksClient               eksiface.EKSAPI
	log                     logr.Logger
	clusterName             string
}

func (p *plan) Create(ctx context.Context) ([]planner.Procedure, error) {
	procedures := []planner.Procedure{}

	if p.desiredIdentityProvider == nil && p.currentIdentityProvider == nil {
		return procedures, nil
	}

	// no config is mentioned deleted provider if we have one
	if p.desiredIdentityProvider == nil {
		// disassociation will also also trigger deletion hence
		// we do nothing in case of ConfigStatusDeleting as it will happen eventually
		if aws.StringValue(p.currentIdentityProvider.Status) == eks.ConfigStatusActive {
			procedures = append(procedures, &DisassociateIdentityProviderConfig{plan: p})
		}

		return procedures, nil
	}

	// create case
	if p.currentIdentityProvider == nil {
		procedures = append(procedures, &AssociateIdentityProviderProcedure{plan: p})
		return procedures, nil
	}

	if p.currentIdentityProvider.IsEqual(p.desiredIdentityProvider) {
		tagsDiff := p.desiredIdentityProvider.Tags.Difference(p.currentIdentityProvider.Tags)
		if len(tagsDiff) > 0 {
			procedures = append(procedures, &UpdatedIdentityProviderTagsProcedure{plan: p})
		}

		if len(p.desiredIdentityProvider.Tags) == 0 && len(p.currentIdentityProvider.Tags) != 0 {
			procedures = append(procedures, &RemoveIdentityProviderTagsProcedure{plan: p})
		}
		switch aws.StringValue(p.currentIdentityProvider.Status) {
		case eks.ConfigStatusActive:
			// config active no work to be done
			return procedures, nil
		case eks.ConfigStatusCreating:
			// no change need wait for association to complete
			procedures = append(procedures, &WaitIdentityProviderAssociatedProcedure{plan: p})
		}
	} else {
		procedures = append(procedures, &DisassociateIdentityProviderConfig{plan: p})
	}

	return procedures, nil
}