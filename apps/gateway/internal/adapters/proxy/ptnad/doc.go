// Package ptnad implements the PT NAD boundary for searches, context, and
// explicit evidence exports. It compiles restricted filters into fixed BQL;
// ordinary records are sanitized metadata. Evidence bytes leave only through
// the explicit export reader. Credentials and vendor task locations stay private.
package ptnad
