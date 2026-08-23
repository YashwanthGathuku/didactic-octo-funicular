package returnrisk

// CategoryAuthorizationTerms identifies return events where an authorization exists
// but the entry is not in accordance with its terms. It is intentionally distinct
// from CategoryUnauthorized even when the return participates in the public
// unauthorized-return-rate monitoring family (for example R11).
const CategoryAuthorizationTerms ReturnCategory = "AUTHORIZATION_TERMS"
