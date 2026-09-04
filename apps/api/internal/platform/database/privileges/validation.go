package privileges

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

type validatedPolicy struct {
	name               string
	requireSchemaUsage bool
	objects            []validatedRule
}

type validatedRule struct {
	kind            ObjectKind
	names           []string
	namePrefix      string
	optional        bool
	required        []Privilege
	forbidden       []Privilege
	publicForbidden []Privilege
	policyName      string
}

var privilegesByKind = map[ObjectKind][]Privilege{
	KindTable: {
		PrivilegeSelect, PrivilegeInsert, PrivilegeUpdate, PrivilegeDelete,
		PrivilegeTruncate, PrivilegeReferences, PrivilegeTrigger,
	},
	KindSequence: {PrivilegeUsage, PrivilegeSelect, PrivilegeUpdate},
	KindFunction: {PrivilegeExecute},
}

func validatePolicies(input []Policy) ([]validatedPolicy, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("at least one policy is required")
	}
	seenPolicyNames := make(map[string]struct{}, len(input))
	validated := make([]validatedPolicy, 0, len(input))
	for policyIndex, policy := range input {
		name := strings.TrimSpace(policy.Name)
		if name == "" {
			return nil, fmt.Errorf("policy %d name is empty", policyIndex+1)
		}
		if name != policy.Name {
			return nil, fmt.Errorf("policy %q name has surrounding whitespace", policy.Name)
		}
		if strings.ContainsRune(name, 0) {
			return nil, fmt.Errorf("policy %q name contains a null byte", name)
		}
		if _, duplicate := seenPolicyNames[name]; duplicate {
			return nil, fmt.Errorf("duplicate policy name %q", name)
		}
		seenPolicyNames[name] = struct{}{}
		if !policy.RequireSchemaUsage && len(policy.Objects) == 0 {
			return nil, fmt.Errorf("policy %q has no rules", name)
		}

		validatedPolicy := validatedPolicy{name: name, requireSchemaUsage: policy.RequireSchemaUsage}
		for ruleIndex, rule := range policy.Objects {
			validatedRule, err := validateRule(name, ruleIndex, rule)
			if err != nil {
				return nil, err
			}
			validatedPolicy.objects = append(validatedPolicy.objects, validatedRule)
		}
		validated = append(validated, validatedPolicy)
	}
	if err := validateStaticConflicts(validated); err != nil {
		return nil, err
	}
	return validated, nil
}

func validateRule(policyName string, ruleIndex int, rule ObjectRule) (validatedRule, error) {
	allowed, supportedKind := privilegesByKind[rule.Kind]
	if !supportedKind {
		return validatedRule{}, fmt.Errorf("policy %q rule %d: unsupported object kind %q", policyName, ruleIndex+1, rule.Kind)
	}
	names, prefix, err := validateRuleSelectors(policyName, ruleIndex, rule)
	if err != nil {
		return validatedRule{}, err
	}
	required, err := validatePrivilegeSet(policyName, ruleIndex, rule.Kind, "Required", rule.Required, allowed)
	if err != nil {
		return validatedRule{}, err
	}
	forbidden, err := validatePrivilegeSet(policyName, ruleIndex, rule.Kind, "Forbidden", rule.Forbidden, allowed)
	if err != nil {
		return validatedRule{}, err
	}
	publicForbidden, err := validatePrivilegeSet(policyName, ruleIndex, rule.Kind, "PublicForbidden", rule.PublicForbidden, allowed)
	if err != nil {
		return validatedRule{}, err
	}
	if len(required)+len(forbidden)+len(publicForbidden) == 0 {
		return validatedRule{}, fmt.Errorf("policy %q rule %d: privilege sets are empty", policyName, ruleIndex+1)
	}
	for _, privilege := range required {
		if slices.Contains(forbidden, privilege) {
			return validatedRule{}, fmt.Errorf(
				"policy %q rule %d: privilege %s is both required and forbidden",
				policyName, ruleIndex+1, privilege,
			)
		}
	}

	return validatedRule{
		kind: rule.Kind, names: names, namePrefix: prefix, optional: rule.Optional,
		required: required, forbidden: forbidden, publicForbidden: publicForbidden,
		policyName: policyName,
	}, nil
}

func validateRuleSelectors(policyName string, ruleIndex int, rule ObjectRule) ([]string, string, error) {
	hasNames := len(rule.Names) > 0
	prefix := strings.TrimSpace(rule.NamePrefix)
	hasPrefix := prefix != ""
	if hasNames == hasPrefix {
		return nil, "", fmt.Errorf("policy %q rule %d: select exactly one of Names or NamePrefix", policyName, ruleIndex+1)
	}
	if prefix != rule.NamePrefix {
		return nil, "", fmt.Errorf("policy %q rule %d: NamePrefix has surrounding whitespace", policyName, ruleIndex+1)
	}
	if strings.ContainsRune(prefix, 0) {
		return nil, "", fmt.Errorf("policy %q rule %d: NamePrefix contains a null byte", policyName, ruleIndex+1)
	}

	names := make([]string, 0, len(rule.Names))
	seenNames := make(map[string]struct{}, len(rule.Names))
	for _, rawName := range rule.Names {
		name := strings.TrimSpace(rawName)
		if name == "" || name != rawName || strings.ContainsRune(name, 0) {
			return nil, "", fmt.Errorf("policy %q rule %d: invalid object name %q", policyName, ruleIndex+1, rawName)
		}
		if rule.Kind == KindFunction {
			functionName, arguments, err := parseFunctionSignature(name)
			if err != nil {
				return nil, "", fmt.Errorf("policy %q rule %d: %w", policyName, ruleIndex+1, err)
			}
			name = functionName + "(" + arguments + ")"
		}
		if _, duplicate := seenNames[name]; duplicate {
			return nil, "", fmt.Errorf("policy %q rule %d: duplicate object name %q", policyName, ruleIndex+1, name)
		}
		seenNames[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, prefix, nil
}

func validatePrivilegeSet(
	policyName string,
	ruleIndex int,
	kind ObjectKind,
	setName string,
	values []Privilege,
	allowed []Privilege,
) ([]Privilege, error) {
	seen := make(map[Privilege]struct{}, len(values))
	for _, privilege := range values {
		if !slices.Contains(allowed, privilege) {
			return nil, fmt.Errorf(
				"policy %q rule %d: unsupported %s privilege for %s",
				policyName, ruleIndex+1, privilege, kind,
			)
		}
		if _, duplicate := seen[privilege]; duplicate {
			return nil, fmt.Errorf("policy %q rule %d: duplicate %s privilege %s", policyName, ruleIndex+1, setName, privilege)
		}
		seen[privilege] = struct{}{}
	}
	normalized := make([]Privilege, 0, len(values))
	for _, privilege := range allowed {
		if _, ok := seen[privilege]; ok {
			normalized = append(normalized, privilege)
		}
	}
	return normalized, nil
}

func validateStaticConflicts(policies []validatedPolicy) error {
	seen := make(map[string]validatedRule)
	for _, policy := range policies {
		for _, rule := range policy.objects {
			selectors := rule.names
			selectorKind := "name"
			if rule.namePrefix != "" {
				selectors = []string{rule.namePrefix}
				selectorKind = "prefix"
			}
			for _, selector := range selectors {
				key := string(rule.kind) + "\x00" + selectorKind + "\x00" + selector
				previous, exists := seen[key]
				if exists && !validatedContractsEqual(previous, rule) {
					return fmt.Errorf(
						"policies %q and %q define conflicting rules for %s %q",
						previous.policyName, rule.policyName, rule.kind, selector,
					)
				}
				if !exists {
					seen[key] = rule
				}
			}
		}
	}
	return nil
}

func validatedContractsEqual(left, right validatedRule) bool {
	return left.optional == right.optional &&
		slices.Equal(left.required, right.required) &&
		slices.Equal(left.forbidden, right.forbidden) &&
		slices.Equal(left.publicForbidden, right.publicForbidden)
}

func parseFunctionSignature(signature string) (string, string, error) {
	open := strings.IndexByte(signature, '(')
	if open <= 0 || !strings.HasSuffix(signature, ")") {
		return "", "", fmt.Errorf("invalid function signature %q", signature)
	}
	name := strings.TrimSpace(signature[:open])
	arguments := strings.TrimSpace(signature[open+1 : len(signature)-1])
	if name == "" || strings.ContainsRune(name, 0) || strings.ContainsRune(arguments, 0) {
		return "", "", fmt.Errorf("invalid function signature %q", signature)
	}
	if strings.ContainsAny(name, "()") {
		return "", "", fmt.Errorf("invalid function signature %q", signature)
	}
	return name, arguments, nil
}
