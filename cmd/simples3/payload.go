package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/rhnvrm/simples3"
)

type keyValueFlag map[string]string

func (f *keyValueFlag) String() string {
	if f == nil {
		return ""
	}
	pairs := make([]string, 0, len(*f))
	for key, value := range *f {
		pairs = append(pairs, key+"="+value)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

func (f *keyValueFlag) Set(value string) error {
	key, val, ok := strings.Cut(value, "=")
	if !ok || key == "" {
		return fmt.Errorf("expected key=value")
	}
	if *f == nil {
		*f = map[string]string{}
	}
	(*f)[key] = val
	return nil
}

type ownerDocument struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

type granteeDocument struct {
	Type         string `json:"type"`
	ID           string `json:"id,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
	URI          string `json:"uri,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty"`
}

type grantDocument struct {
	Grantee    granteeDocument `json:"grantee"`
	Permission string          `json:"permission"`
}

type accessControlPolicyDocument struct {
	Owner  ownerDocument   `json:"owner"`
	Grants []grantDocument `json:"grants"`
}

type tagDocument struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type lifecycleAndDocument struct {
	Prefix string        `json:"prefix,omitempty"`
	Tags   []tagDocument `json:"tags,omitempty"`
}

type lifecycleFilterDocument struct {
	Prefix string                `json:"prefix,omitempty"`
	Tag    *tagDocument          `json:"tag,omitempty"`
	And    *lifecycleAndDocument `json:"and,omitempty"`
}

type lifecycleExpirationDocument struct {
	Date                      string `json:"date,omitempty"`
	Days                      int    `json:"days,omitempty"`
	ExpiredObjectDeleteMarker bool   `json:"expiredObjectDeleteMarker,omitempty"`
}

type lifecycleTransitionDocument struct {
	Date         string `json:"date,omitempty"`
	Days         int    `json:"days,omitempty"`
	StorageClass string `json:"storageClass"`
}

type lifecycleNoncurrentVersionExpirationDocument struct {
	NoncurrentDays int `json:"noncurrentDays"`
}

type lifecycleNoncurrentVersionTransitionDocument struct {
	NoncurrentDays int    `json:"noncurrentDays"`
	StorageClass   string `json:"storageClass"`
}

type lifecycleAbortIncompleteMultipartUploadDocument struct {
	DaysAfterInitiation int `json:"daysAfterInitiation"`
}

type lifecycleRuleDocument struct {
	ID                             string                                           `json:"id,omitempty"`
	Status                         string                                           `json:"status"`
	Filter                         *lifecycleFilterDocument                         `json:"filter,omitempty"`
	Prefix                         *string                                          `json:"prefix,omitempty"`
	Expiration                     *lifecycleExpirationDocument                     `json:"expiration,omitempty"`
	Transitions                    []lifecycleTransitionDocument                    `json:"transitions,omitempty"`
	NoncurrentVersionExpiration    *lifecycleNoncurrentVersionExpirationDocument    `json:"noncurrentVersionExpiration,omitempty"`
	NoncurrentVersionTransitions   []lifecycleNoncurrentVersionTransitionDocument   `json:"noncurrentVersionTransitions,omitempty"`
	AbortIncompleteMultipartUpload *lifecycleAbortIncompleteMultipartUploadDocument `json:"abortIncompleteMultipartUpload,omitempty"`
}

type lifecycleConfigurationDocument struct {
	Rules []lifecycleRuleDocument `json:"rules"`
}

func ownerDocumentFromOwner(owner simples3.Owner) ownerDocument {
	return ownerDocument{ID: owner.ID, DisplayName: owner.DisplayName}
}

func (doc ownerDocument) toOwner() simples3.Owner {
	return simples3.Owner{ID: doc.ID, DisplayName: doc.DisplayName}
}

func granteeDocumentFromGrantee(grantee simples3.Grantee) granteeDocument {
	return granteeDocument{
		Type:         grantee.Type,
		ID:           grantee.ID,
		DisplayName:  grantee.DisplayName,
		URI:          grantee.URI,
		EmailAddress: grantee.EmailAddress,
	}
}

func (doc granteeDocument) toGrantee() simples3.Grantee {
	return simples3.Grantee{
		Type:         doc.Type,
		ID:           doc.ID,
		DisplayName:  doc.DisplayName,
		URI:          doc.URI,
		EmailAddress: doc.EmailAddress,
	}
}

func grantDocumentFromGrant(grant simples3.Grant) grantDocument {
	return grantDocument{Grantee: granteeDocumentFromGrantee(grant.Grantee), Permission: grant.Permission}
}

func (doc grantDocument) toGrant() simples3.Grant {
	return simples3.Grant{Grantee: doc.Grantee.toGrantee(), Permission: doc.Permission}
}

func aclDocumentFromPolicy(policy simples3.AccessControlPolicy) accessControlPolicyDocument {
	grants := make([]grantDocument, 0, len(policy.AccessControlList))
	for _, grant := range policy.AccessControlList {
		grants = append(grants, grantDocumentFromGrant(grant))
	}
	return accessControlPolicyDocument{Owner: ownerDocumentFromOwner(policy.Owner), Grants: grants}
}

func (doc accessControlPolicyDocument) toPolicy() *simples3.AccessControlPolicy {
	grants := make([]simples3.Grant, 0, len(doc.Grants))
	for _, grant := range doc.Grants {
		grants = append(grants, grant.toGrant())
	}
	return &simples3.AccessControlPolicy{Owner: doc.Owner.toOwner(), AccessControlList: grants}
}

func tagDocumentFromTag(tag simples3.Tag) tagDocument {
	return tagDocument{Key: tag.Key, Value: tag.Value}
}

func (doc tagDocument) toTag() simples3.Tag {
	return simples3.Tag{Key: doc.Key, Value: doc.Value}
}

func lifecycleFilterDocumentFromFilter(filter *simples3.LifecycleFilter) *lifecycleFilterDocument {
	if filter == nil {
		return nil
	}
	doc := &lifecycleFilterDocument{Prefix: filter.Prefix}
	if filter.Tag != nil {
		tag := tagDocumentFromTag(*filter.Tag)
		doc.Tag = &tag
	}
	if filter.And != nil {
		and := &lifecycleAndDocument{Prefix: filter.And.Prefix, Tags: make([]tagDocument, 0, len(filter.And.Tags))}
		for _, tag := range filter.And.Tags {
			and.Tags = append(and.Tags, tagDocumentFromTag(tag))
		}
		doc.And = and
	}
	return doc
}

func (doc *lifecycleFilterDocument) toFilter() *simples3.LifecycleFilter {
	if doc == nil {
		return nil
	}
	filter := &simples3.LifecycleFilter{Prefix: doc.Prefix}
	if doc.Tag != nil {
		tag := doc.Tag.toTag()
		filter.Tag = &tag
	}
	if doc.And != nil {
		and := &struct {
			Prefix string         `xml:"Prefix,omitempty"`
			Tags   []simples3.Tag `xml:"Tag,omitempty"`
		}{
			Prefix: doc.And.Prefix,
			Tags:   make([]simples3.Tag, 0, len(doc.And.Tags)),
		}
		for _, tag := range doc.And.Tags {
			and.Tags = append(and.Tags, tag.toTag())
		}
		filter.And = and
	}
	return filter
}

func lifecycleRuleDocumentFromRule(rule simples3.LifecycleRule) lifecycleRuleDocument {
	doc := lifecycleRuleDocument{
		ID:                             rule.ID,
		Status:                         rule.Status,
		Filter:                         lifecycleFilterDocumentFromFilter(rule.Filter),
		Prefix:                         rule.Prefix,
		Transitions:                    make([]lifecycleTransitionDocument, 0, len(rule.Transitions)),
		NoncurrentVersionTransitions:   make([]lifecycleNoncurrentVersionTransitionDocument, 0, len(rule.NoncurrentVersionTransitions)),
		AbortIncompleteMultipartUpload: nil,
	}
	if rule.Expiration != nil {
		doc.Expiration = &lifecycleExpirationDocument{
			Date:                      rule.Expiration.Date,
			Days:                      rule.Expiration.Days,
			ExpiredObjectDeleteMarker: rule.Expiration.ExpiredObjectDeleteMarker,
		}
	}
	for _, transition := range rule.Transitions {
		doc.Transitions = append(doc.Transitions, lifecycleTransitionDocument{
			Date:         transition.Date,
			Days:         transition.Days,
			StorageClass: transition.StorageClass,
		})
	}
	if rule.NoncurrentVersionExpiration != nil {
		doc.NoncurrentVersionExpiration = &lifecycleNoncurrentVersionExpirationDocument{NoncurrentDays: rule.NoncurrentVersionExpiration.NoncurrentDays}
	}
	for _, transition := range rule.NoncurrentVersionTransitions {
		doc.NoncurrentVersionTransitions = append(doc.NoncurrentVersionTransitions, lifecycleNoncurrentVersionTransitionDocument{
			NoncurrentDays: transition.NoncurrentDays,
			StorageClass:   transition.StorageClass,
		})
	}
	if rule.AbortIncompleteMultipartUpload != nil {
		doc.AbortIncompleteMultipartUpload = &lifecycleAbortIncompleteMultipartUploadDocument{DaysAfterInitiation: rule.AbortIncompleteMultipartUpload.DaysAfterInitiation}
	}
	return doc
}

func (doc lifecycleRuleDocument) toRule() simples3.LifecycleRule {
	rule := simples3.LifecycleRule{
		ID:                           doc.ID,
		Status:                       doc.Status,
		Filter:                       doc.Filter.toFilter(),
		Prefix:                       doc.Prefix,
		Transitions:                  make([]simples3.LifecycleTransition, 0, len(doc.Transitions)),
		NoncurrentVersionTransitions: make([]simples3.LifecycleNoncurrentVersionTransition, 0, len(doc.NoncurrentVersionTransitions)),
	}
	if doc.Expiration != nil {
		rule.Expiration = &simples3.LifecycleExpiration{
			Date:                      doc.Expiration.Date,
			Days:                      doc.Expiration.Days,
			ExpiredObjectDeleteMarker: doc.Expiration.ExpiredObjectDeleteMarker,
		}
	}
	for _, transition := range doc.Transitions {
		rule.Transitions = append(rule.Transitions, simples3.LifecycleTransition{
			Date:         transition.Date,
			Days:         transition.Days,
			StorageClass: transition.StorageClass,
		})
	}
	if doc.NoncurrentVersionExpiration != nil {
		rule.NoncurrentVersionExpiration = &simples3.LifecycleNoncurrentVersionExpiration{NoncurrentDays: doc.NoncurrentVersionExpiration.NoncurrentDays}
	}
	for _, transition := range doc.NoncurrentVersionTransitions {
		rule.NoncurrentVersionTransitions = append(rule.NoncurrentVersionTransitions, simples3.LifecycleNoncurrentVersionTransition{
			NoncurrentDays: transition.NoncurrentDays,
			StorageClass:   transition.StorageClass,
		})
	}
	if doc.AbortIncompleteMultipartUpload != nil {
		rule.AbortIncompleteMultipartUpload = &simples3.LifecycleAbortIncompleteMultipartUpload{DaysAfterInitiation: doc.AbortIncompleteMultipartUpload.DaysAfterInitiation}
	}
	return rule
}

func lifecycleDocumentFromConfiguration(config simples3.LifecycleConfiguration) lifecycleConfigurationDocument {
	rules := make([]lifecycleRuleDocument, 0, len(config.Rules))
	for _, rule := range config.Rules {
		rules = append(rules, lifecycleRuleDocumentFromRule(rule))
	}
	return lifecycleConfigurationDocument{Rules: rules}
}

func (doc lifecycleConfigurationDocument) toConfiguration() *simples3.LifecycleConfiguration {
	rules := make([]simples3.LifecycleRule, 0, len(doc.Rules))
	for _, rule := range doc.Rules {
		rules = append(rules, rule.toRule())
	}
	return &simples3.LifecycleConfiguration{Rules: rules}
}

func decodeJSONSource(rt *runtime, path string, target any) error {
	reader, closer, err := openInputSource(rt, path)
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return usageErrorf("invalid JSON in %q: %v", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return usageErrorf("invalid JSON in %q: multiple top-level values", path)
		}
		return usageErrorf("invalid JSON in %q: %v", path, err)
	}
	return nil
}

func openInputSource(rt *runtime, path string) (io.Reader, io.Closer, error) {
	if path == "-" {
		if rt.stdin == nil {
			return nil, nil, usageErrorf("stdin is not available")
		}
		return rt.stdin, nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return file, file, nil
}

func writeTagLines(out io.Writer, tags map[string]string) {
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		printLine(out, "%s=%s", key, tags[key])
	}
}
