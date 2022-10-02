package grpcroutetranslation

import (
	"fmt"

	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func methodMatchToPathMatch(matchtype string, service *string, method *string) (urimatchtype string, uri string, err error) {
	if matchtype == "Exact" {
		if service != nil {
			if method != nil {
				urimatchtype = "Exact"
				uri = fmt.Sprintf("/%s/%s", *service, *method)
				return
			} else { // method == nil
				urimatchtype = "PathPrefix"
				uri = fmt.Sprintf("/%s/", *service)
				return
			}
		} else { // service == nil
			if method != nil {
				urimatchtype = "RegularExpression"
				uri = fmt.Sprintf("/.+/%s", *method)
				return
			} else { // method == nil
				err = fmt.Errorf("GRPCRoute Method match specified but neither service nor method specified.")
				return
			}
		}
	} else { // matchtype == "RegularExpression"
		urimatchtype = "RegularExpression"
		if service != nil {
			if method != nil {
				urimatchtype = fmt.Sprintf("/%s/%s", *service, *method)
				return
			} else { // method == nil
				urimatchtype = fmt.Sprintf("/%s/.+", *service)
				return
			}
		} else { // service == nil
			if method != nil {
				urimatchtype = fmt.Sprintf("/.+/%s", *method)
				return
			} else { // method == nil
				err = fmt.Errorf("GRPCRoute Method match specified but neither service nor method specified.")
				return
			}
		}
	}
	return
}

func methodMatcherToPathMatcher(inmethod v1alpha2.GRPCMethodMatch) (outmatch *v1beta1.HTTPPathMatch, err error) {
	var matchtype v1alpha2.GRPCMethodMatchType = "Exact"
	if inmethod.Type != nil {
		intype := *inmethod.Type
		// TODO: Maybe maintain a list and iterate over it?
		if intype == v1alpha2.GRPCMethodMatchType("Exact") || intype == v1alpha2.GRPCMethodMatchType("RegularExpression") {
			matchtype = intype
		} else {
			err = fmt.Errorf("Unsupported GRPCRoute MethodMatch type %s", intype)
		}
	}

	urimatchtype, uri, matcherr := methodMatchToPathMatch(string(matchtype), inmethod.Service, inmethod.Method)
	if matcherr != nil {
		err = matcherr
		return
	}
	outmatchtype := v1beta1.PathMatchType(urimatchtype)
	outmatch = &v1beta1.HTTPPathMatch{
		Type: &outmatchtype,
		Value: &uri,
	}
	return
}

// TODO: Use something besides fmt.Errorf?
// TODO: Maybe take pointers instead? Whatever seems more natural for the caller.
func TranslateGRPCRoute(gr v1alpha2.GRPCRouteSpec) (hr v1beta1.HTTPRouteSpec, err error) {
	for _, h := range(gr.Hostnames) {
		hr.Hostnames = append(hr.Hostnames, h)
	}

	for _, pr := range(gr.ParentRefs) {
		hr.ParentRefs = append(hr.ParentRefs, pr)
	}

	// TODO: Probably break this out into internal functions.
	for _, inrule := range(gr.Rules) {
		outrule := v1beta1.HTTPRouteRule{}
		for _, inmatch := range(inrule.Matches) {
			outmatch := v1beta1.HTTPRouteMatch{}
			if inmatch.Method != nil {
				outmatch.Path, err = methodMatcherToPathMatcher(*inmatch.Method)
				if err != nil {
					return
				}
			}
			// TODO: Headers
			for _, headerin := range(inmatch.Headers) {
				headerout := v1beta1.HTTPHeaderMatch{
					Type: headerin.Type,
					Name: v1beta1.HTTPHeaderName(headerin.Name),
					Value: headerin.Value,
				}
				outmatch.Headers = append(outmatch.Headers, headerout)
			}
			outrule.Matches = append(outrule.Matches, outmatch)
		}
		// TODO: Filters.
		outbrs := []v1beta1.HTTPBackendRef{}
		for _, inbr := range(inrule.BackendRefs) {
			// TODO: Do something with the filters.
			outbr := v1beta1.HTTPBackendRef{}
			outbr.BackendObjectReference = inbr.BackendRefs.BackendObjectReference
			outbr.Weight = inbr.BackendRefs.Weight
			outbrs = append(outbrs, outbr)
		}
		hr.Rules = append(hr.Rules, outrule)
	}
	return
}
