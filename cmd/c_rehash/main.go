// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2025 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

// c_rehash scans a directory for X.509 certificate and CRL files and creates
// symbolic links named by the subject (or issuer) hash, matching the naming
// scheme used by OpenSSL's c_rehash tool.  The hash and link-name format are
// compatible with OpenSSL's new-style subject_hash (the default since OpenSSL
// 1.0.0).
//
// Usage: c_rehash <directory>
package main

import (
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf16"
)

// hashLinkRe matches the symlink names created by c_rehash:
// eight lower-case hex digits, a dot, an optional "r" (CRL), and digits.
var hashLinkRe = regexp.MustCompile(`^[0-9a-f]{8}\.r?\d+$`)

// tagUniversalString is the ASN.1 tag for UniversalString (UTF-32 BE).
// It is not exported by encoding/asn1.
const tagUniversalString = 28

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: c_rehash <directory>\n")
		os.Exit(1)
	}
	if err := rehashDir(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "c_rehash: %v\n", err)
		os.Exit(1)
	}
}

// rehashDir removes stale hash symlinks from dir and creates new ones for
// every certificate and CRL file found there.
func rehashDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("cannot read directory: %w", err)
	}

	// Remove existing hash symlinks so that renamed/deleted certs do not
	// leave stale links behind.
	for _, e := range entries {
		name := e.Name()
		if !hashLinkRe.MatchString(name) {
			continue
		}
		path := filepath.Join(dir, name)
		fi, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("cannot remove %q: %w", path, err)
		}
	}

	// hashlist maps "<hash>.<crlmark><suffix>" to the fingerprint of the
	// file already linked there.  It is used to detect duplicates and to
	// pick the next free suffix.
	hashlist := make(map[string]string)

	for _, e := range entries {
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".pem", ".crt", ".cer", ".crl":
		default:
			continue
		}

		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: cannot read %q: %v\n", path, err)
			continue
		}

		isCert, isCRL := detectTypes(data)
		if !isCert && !isCRL {
			fmt.Fprintf(os.Stderr, "WARNING: %s does not contain a certificate or CRL: skipping\n", name)
			continue
		}
		if isCert {
			if err := linkHash(dir, name, data, false, hashlist); err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: %v\n", err)
			}
		}
		if isCRL {
			if err := linkHash(dir, name, data, true, hashlist); err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: %v\n", err)
			}
		}
	}
	return nil
}

// detectTypes scans the PEM blocks in data and reports whether a certificate
// or a CRL is present.  It matches the same header strings as the Perl
// c_rehash script.
func detectTypes(data []byte) (isCert, isCRL bool) {
	rest := data
	for len(rest) > 0 {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		switch block.Type {
		case "CERTIFICATE", "X509 CERTIFICATE", "TRUSTED CERTIFICATE":
			isCert = true
		case "X509 CRL":
			isCRL = true
		}
		if isCert && isCRL {
			break
		}
	}
	return
}

// linkHash computes the hash and fingerprint for the first certificate or CRL
// block in data, then creates a symlink <hash>.<crlmark><suffix> → fname
// inside dir, skipping duplicates.
func linkHash(dir, fname string, data []byte, isCRL bool, hashlist map[string]string) error {
	var hash, fprint string
	var err error
	if isCRL {
		hash, fprint, err = computeCRLHash(data)
	} else {
		hash, fprint, err = computeCertHash(data)
	}
	if err != nil {
		return fmt.Errorf("cannot compute hash for %q: %w", fname, err)
	}

	crlMark := ""
	if isCRL {
		crlMark = "r"
	}

	for suffix := 0; ; suffix++ {
		key := fmt.Sprintf("%s.%s%d", hash, crlMark, suffix)
		existing, exists := hashlist[key]
		if !exists {
			linkPath := filepath.Join(dir, key)
			if err := os.Symlink(fname, linkPath); err != nil {
				return fmt.Errorf("cannot create symlink %q -> %q: %w", key, fname, err)
			}
			hashlist[key] = fprint
			return nil
		}
		if existing == fprint {
			what := "certificate"
			if isCRL {
				what = "CRL"
			}
			fmt.Fprintf(os.Stderr, "WARNING: skipping duplicate %s %q\n", what, fname)
			return nil
		}
	}
}

// computeCertHash returns the new-style OpenSSL subject hash and the SHA-1
// fingerprint (hex, no colons) of the first certificate PEM block in data.
func computeCertHash(data []byte) (hash, fprint string, err error) {
	block := firstPEMBlock(data, "CERTIFICATE", "X509 CERTIFICATE", "TRUSTED CERTIFICATE")
	if block == nil {
		return "", "", fmt.Errorf("no certificate PEM block found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("cannot parse certificate: %w", err)
	}
	hash, err = subjectHash(cert.RawSubject)
	if err != nil {
		return "", "", err
	}
	h := sha1.Sum(cert.Raw)
	fprint = hex.EncodeToString(h[:])
	return
}

// tbsCertListPartial is a minimal view of a TBSCertList used solely to
// extract the raw DER bytes of the Issuer field.  Extra fields after
// Issuer are silently ignored by encoding/asn1.
type tbsCertListPartial struct {
	Version   int                      `asn1:"optional,default:0"`
	Signature pkix.AlgorithmIdentifier
	Issuer    asn1.RawValue
}

// crlPartial is a minimal view of a CRL used solely to reach
// tbsCertListPartial.
type crlPartial struct {
	TBSCertList tbsCertListPartial
}

// computeCRLHash returns the new-style OpenSSL issuer hash and the SHA-1
// fingerprint (hex, no colons) of the first CRL PEM block in data.
func computeCRLHash(data []byte) (hash, fprint string, err error) {
	block := firstPEMBlock(data, "X509 CRL")
	if block == nil {
		return "", "", fmt.Errorf("no CRL PEM block found")
	}
	var crl crlPartial
	if _, err := asn1.Unmarshal(block.Bytes, &crl); err != nil {
		return "", "", fmt.Errorf("cannot parse CRL: %w", err)
	}
	rawIssuer := crl.TBSCertList.Issuer.FullBytes
	if len(rawIssuer) == 0 {
		return "", "", fmt.Errorf("empty CRL issuer")
	}
	hash, err = subjectHash(rawIssuer)
	if err != nil {
		return "", "", err
	}
	h := sha1.Sum(block.Bytes)
	fprint = hex.EncodeToString(h[:])
	return
}

// firstPEMBlock returns the first PEM block in data whose Type is one of the
// given types, or nil if none is found.
func firstPEMBlock(data []byte, types ...string) *pem.Block {
	want := make(map[string]bool, len(types))
	for _, t := range types {
		want[t] = true
	}
	rest := data
	for len(rest) > 0 {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if want[block.Type] {
			return block
		}
	}
	return nil
}

// subjectHash computes the new-style OpenSSL subject hash for rawSubject (the
// DER-encoded RDNSequence).  It canonicalises the name, then computes a SHA-1
// digest of the inner bytes of the resulting SEQUENCE (i.e. without the outer
// tag and length), and returns the first four bytes interpreted as a
// little-endian uint32 formatted as eight lower-case hex digits.
//
// This matches OpenSSL's X509_NAME_hash / x509_name_canon algorithm.
func subjectHash(rawSubject []byte) (string, error) {
	canonical, err := canonicalizeSubject(rawSubject)
	if err != nil {
		return "", fmt.Errorf("cannot canonicalize subject: %w", err)
	}
	// OpenSSL hashes the *inner* bytes of the SEQUENCE (after stripping the
	// outer SEQUENCE tag and length), not the full DER.
	var outer asn1.RawValue
	if _, err := asn1.Unmarshal(canonical, &outer); err != nil {
		return "", fmt.Errorf("cannot unwrap canonical subject: %w", err)
	}
	h := sha1.Sum(outer.Bytes)
	v := uint32(h[0]) | uint32(h[1])<<8 | uint32(h[2])<<16 | uint32(h[3])<<24
	return fmt.Sprintf("%08x", v), nil
}

// canonicalizeSubject returns the DER encoding of rawSubject after applying
// OpenSSL's X509_NAME_hash canonicalisation:
//
//   - every ASN.1 string attribute value is decoded to UTF-8, lower-cased,
//     stripped of leading/trailing whitespace, and internal whitespace runs
//     are collapsed to a single space;
//   - the resulting string is re-encoded as a UTF8String.
//
// The outer SEQUENCE and inner SET tags are preserved verbatim so that the
// result is structurally identical to what OpenSSL produces.
func canonicalizeSubject(rawSubject []byte) ([]byte, error) {
	var outer asn1.RawValue
	if _, err := asn1.Unmarshal(rawSubject, &outer); err != nil {
		return nil, err
	}

	var canonSets []byte
	rest := outer.Bytes
	for len(rest) > 0 {
		var rdnSet asn1.RawValue
		var err error
		rest, err = asn1.Unmarshal(rest, &rdnSet)
		if err != nil {
			return nil, err
		}

		var canonAttrs []byte
		attrRest := rdnSet.Bytes
		for len(attrRest) > 0 {
			var attrSeq asn1.RawValue
			var err error
			attrRest, err = asn1.Unmarshal(attrRest, &attrSeq)
			if err != nil {
				return nil, err
			}

			// Each attribute is SEQUENCE { OID, value }.
			var oid asn1.ObjectIdentifier
			valRest, err := asn1.Unmarshal(attrSeq.Bytes, &oid)
			if err != nil {
				return nil, err
			}
			var val asn1.RawValue
			if _, err = asn1.Unmarshal(valRest, &val); err != nil {
				return nil, err
			}

			if isASN1StringTag(val.Tag) {
				if s, err := decodeASN1String(val); err == nil {
					s = collapseWhitespace(strings.ToLower(strings.TrimSpace(s)))
					val = asn1.RawValue{
						Class:      asn1.ClassUniversal,
						Tag:        asn1.TagUTF8String,
						IsCompound: false,
						Bytes:      []byte(s),
					}
				}
			}

			oidDER, err := asn1.Marshal(oid)
			if err != nil {
				return nil, err
			}
			valDER, err := asn1.Marshal(val)
			if err != nil {
				return nil, err
			}
			attrDER, err := asn1.Marshal(asn1.RawValue{
				Class:      asn1.ClassUniversal,
				Tag:        asn1.TagSequence,
				IsCompound: true,
				Bytes:      append(oidDER, valDER...),
			})
			if err != nil {
				return nil, err
			}
			canonAttrs = append(canonAttrs, attrDER...)
		}

		setDER, err := asn1.Marshal(asn1.RawValue{
			Class:      asn1.ClassUniversal,
			Tag:        asn1.TagSet,
			IsCompound: true,
			Bytes:      canonAttrs,
		})
		if err != nil {
			return nil, err
		}
		canonSets = append(canonSets, setDER...)
	}

	return asn1.Marshal(asn1.RawValue{
		Class:      asn1.ClassUniversal,
		Tag:        asn1.TagSequence,
		IsCompound: true,
		Bytes:      canonSets,
	})
}

// isASN1StringTag reports whether tag is one of the ASN.1 string types that
// OpenSSL canonicalises.
func isASN1StringTag(tag int) bool {
	switch tag {
	case asn1.TagUTF8String,
		asn1.TagNumericString,
		asn1.TagPrintableString,
		asn1.TagT61String,
		asn1.TagIA5String,
		asn1.TagGeneralString,
		asn1.TagBMPString,
		tagUniversalString:
		return true
	}
	return false
}

// decodeASN1String decodes an ASN.1 string RawValue to a Go UTF-8 string.
// Supported encodings: UTF-8, Numeric, Printable, IA5, General (direct byte
// mapping), T61/Teletex (Latin-1), BMP (UTF-16 BE), Universal (UTF-32 BE).
func decodeASN1String(val asn1.RawValue) (string, error) {
	switch val.Tag {
	case asn1.TagUTF8String,
		asn1.TagNumericString,
		asn1.TagPrintableString,
		asn1.TagIA5String,
		asn1.TagGeneralString:
		return string(val.Bytes), nil

	case asn1.TagT61String:
		// T61String / TeletexString: treat each byte as a Latin-1 code point.
		runes := make([]rune, len(val.Bytes))
		for i, b := range val.Bytes {
			runes[i] = rune(b)
		}
		return string(runes), nil

	case asn1.TagBMPString:
		if len(val.Bytes)%2 != 0 {
			return "", fmt.Errorf("BMPString has odd byte count %d", len(val.Bytes))
		}
		u16 := make([]uint16, len(val.Bytes)/2)
		for i := range u16 {
			u16[i] = uint16(val.Bytes[2*i])<<8 | uint16(val.Bytes[2*i+1])
		}
		return string(utf16.Decode(u16)), nil

	case tagUniversalString:
		if len(val.Bytes)%4 != 0 {
			return "", fmt.Errorf("UniversalString byte count %d is not a multiple of 4", len(val.Bytes))
		}
		runes := make([]rune, len(val.Bytes)/4)
		for i := range runes {
			runes[i] = rune(val.Bytes[4*i])<<24 |
				rune(val.Bytes[4*i+1])<<16 |
				rune(val.Bytes[4*i+2])<<8 |
				rune(val.Bytes[4*i+3])
		}
		return string(runes), nil
	}
	return "", fmt.Errorf("unsupported ASN.1 string tag %d", val.Tag)
}

// collapseWhitespace returns s with every run of Unicode whitespace replaced
// by a single ASCII space.
func collapseWhitespace(s string) string {
	var b strings.Builder
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace {
				b.WriteRune(' ')
				inSpace = true
			}
		} else {
			b.WriteRune(r)
			inSpace = false
		}
	}
	return b.String()
}
