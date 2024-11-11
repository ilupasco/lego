package cmd

import (
	"crypto"
	"crypto/x509"
	"errors"
	"math/rand"
	"os"
	"time"

	"github.com/go-acme/lego/v4/acme/api"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/log"
	"github.com/mattn/go-isatty"
	"github.com/urfave/cli/v2"
)

// Flag names.
const (
	flgDays                   = "days"
	flgARIDisable             = "ari-disable"
	flgARIWaitToRenewDuration = "ari-wait-to-renew-duration"
	flgReuseKey               = "reuse-key"
	flgRenewHook              = "renew-hook"
	flgNoRandomSleep          = "no-random-sleep"
)

const (
	renewEnvAccountEmail      = "LEGO_ACCOUNT_EMAIL"
	renewEnvCertDomain        = "LEGO_CERT_DOMAIN"
	renewEnvCertPath          = "LEGO_CERT_PATH"
	renewEnvCertKeyPath       = "LEGO_CERT_KEY_PATH"
	renewEnvIssuerCertKeyPath = "LEGO_ISSUER_CERT_PATH"
	renewEnvCertPEMPath       = "LEGO_CERT_PEM_PATH"
	renewEnvCertPFXPath       = "LEGO_CERT_PFX_PATH"
)

func createRenew() *cli.Command {
	return &cli.Command{
		Name:   "renew",
		Usage:  "Renew a certificate",
		Action: renew,
		Before: func(ctx *cli.Context) error {
			// we require either domains or csr, but not both
			hasDomains := len(ctx.StringSlice(flgDomains)) > 0
			hasCsr := ctx.String(flgCSR) != ""
			if hasDomains && hasCsr {
				log.Fatal("Please specify either --%s/-d or --%s/-c, but not both", flgDomains, flgCSR)
			}
			if !hasDomains && !hasCsr {
				log.Fatal("Please specify --%s/-d (or --%s/-c if you already have a CSR)", flgDomains, flgCSR)
			}
			return nil
		},
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  flgDays,
				Value: 30,
				Usage: "The number of days left on a certificate to renew it.",
			},
			&cli.BoolFlag{
				Name:  flgARIDisable,
				Usage: "Do not use the renewalInfo endpoint (draft-ietf-acme-ari) to check if a certificate should be renewed.",
			},
			&cli.DurationFlag{
				Name:  flgARIWaitToRenewDuration,
				Usage: "The maximum duration you're willing to sleep for a renewal time returned by the renewalInfo endpoint.",
			},
			&cli.BoolFlag{
				Name:  flgReuseKey,
				Usage: "Used to indicate you want to reuse your current private key for the new certificate.",
			},
			&cli.BoolFlag{
				Name:  flgNoBundle,
				Usage: "Do not create a certificate bundle by adding the issuers certificate to the new certificate.",
			},
			&cli.BoolFlag{
				Name: flgMustStaple,
				Usage: "Include the OCSP must staple TLS extension in the CSR and generated certificate." +
					" Only works if the CSR is generated by lego.",
			},
			&cli.TimestampFlag{
				Name:   flgNotBefore,
				Usage:  "Set the notBefore field in the certificate (RFC3339 format)",
				Layout: time.RFC3339,
			},
			&cli.TimestampFlag{
				Name:   flgNotAfter,
				Usage:  "Set the notAfter field in the certificate (RFC3339 format)",
				Layout: time.RFC3339,
			},
			&cli.StringFlag{
				Name: flgPreferredChain,
				Usage: "If the CA offers multiple certificate chains, prefer the chain with an issuer matching this Subject Common Name." +
					" If no match, the default offered chain will be used.",
			},
			&cli.StringFlag{
				Name:  flgAlwaysDeactivateAuthorizations,
				Usage: "Force the authorizations to be relinquished even if the certificate request was successful.",
			},
			&cli.StringFlag{
				Name:  flgRenewHook,
				Usage: "Define a hook. The hook is executed only when the certificates are effectively renewed.",
			},
			&cli.BoolFlag{
				Name: flgNoRandomSleep,
				Usage: "Do not add a random sleep before the renewal." +
					" We do not recommend using this flag if you are doing your renewals in an automated way.",
			},
		},
	}
}

func renew(ctx *cli.Context) error {
	account, client := setup(ctx, NewAccountsStorage(ctx))
	setupChallenges(ctx, client)

	if account.Registration == nil {
		log.Fatalf("Account %s is not registered. Use 'run' to register a new account.\n", account.Email)
	}

	certsStorage := NewCertificatesStorage(ctx)

	bundle := !ctx.Bool(flgNoBundle)

	meta := map[string]string{renewEnvAccountEmail: account.Email}

	// CSR
	if ctx.IsSet(flgCSR) {
		return renewForCSR(ctx, client, certsStorage, bundle, meta)
	}

	// Domains
	return renewForDomains(ctx, client, certsStorage, bundle, meta)
}

func renewForDomains(ctx *cli.Context, client *lego.Client, certsStorage *CertificatesStorage, bundle bool, meta map[string]string) error {
	domains := ctx.StringSlice(flgDomains)
	domain := domains[0]

	// load the cert resource from files.
	// We store the certificate, private key and metadata in different files
	// as web servers would not be able to work with a combined file.
	certificates, err := certsStorage.ReadCertificate(domain, certExt)
	if err != nil {
		log.Fatalf("Error while loading the certificate for domain %s\n\t%v", domain, err)
	}

	cert := certificates[0]

	var ariRenewalTime *time.Time
	var replacesCertID string

	if !ctx.Bool(flgARIDisable) {
		ariRenewalTime = getARIRenewalTime(ctx, cert, domain, client)
		if ariRenewalTime != nil {
			now := time.Now().UTC()

			// Figure out if we need to sleep before renewing.
			if ariRenewalTime.After(now) {
				log.Infof("[%s] Sleeping %s until renewal time %s", domain, ariRenewalTime.Sub(now), ariRenewalTime)
				time.Sleep(ariRenewalTime.Sub(now))
			}
		}

		replacesCertID, err = certificate.MakeARICertID(cert)
		if err != nil {
			log.Fatalf("Error while construction the ARI CertID for domain %s\n\t%v", domain, err)
		}
	}

	if ariRenewalTime == nil && !needRenewal(cert, domain, ctx.Int(flgDays)) {
		return nil
	}

	// This is just meant to be informal for the user.
	timeLeft := cert.NotAfter.Sub(time.Now().UTC())
	log.Infof("[%s] acme: Trying renewal with %d hours remaining", domain, int(timeLeft.Hours()))

	certDomains := certcrypto.ExtractDomains(cert)

	var privateKey crypto.PrivateKey
	if ctx.Bool(flgReuseKey) {
		keyBytes, errR := certsStorage.ReadFile(domain, keyExt)
		if errR != nil {
			log.Fatalf("Error while loading the private key for domain %s\n\t%v", domain, errR)
		}

		privateKey, errR = certcrypto.ParsePEMPrivateKey(keyBytes)
		if errR != nil {
			return errR
		}
	}

	// https://github.com/go-acme/lego/issues/1656
	// https://github.com/certbot/certbot/blob/284023a1b7672be2bd4018dd7623b3b92197d4b0/certbot/certbot/_internal/renewal.py#L435-L440
	if !isatty.IsTerminal(os.Stdout.Fd()) && !ctx.Bool(flgNoRandomSleep) {
		// https://github.com/certbot/certbot/blob/284023a1b7672be2bd4018dd7623b3b92197d4b0/certbot/certbot/_internal/renewal.py#L472
		const jitter = 8 * time.Minute
		rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
		sleepTime := time.Duration(rnd.Int63n(int64(jitter)))

		log.Infof("renewal: random delay of %s", sleepTime)
		time.Sleep(sleepTime)
	}

	request := certificate.ObtainRequest{
		Domains:                        merge(certDomains, domains),
		PrivateKey:                     privateKey,
		MustStaple:                     ctx.Bool(flgMustStaple),
		NotBefore:                      getTime(ctx, flgNotBefore),
		NotAfter:                       getTime(ctx, flgNotAfter),
		Bundle:                         bundle,
		PreferredChain:                 ctx.String(flgPreferredChain),
		AlwaysDeactivateAuthorizations: ctx.Bool(flgAlwaysDeactivateAuthorizations),
	}

	if replacesCertID != "" {
		request.ReplacesCertID = replacesCertID
	}

	certRes, err := client.Certificate.Obtain(request)
	if err != nil {
		log.Fatal(err)
	}

	certsStorage.SaveResource(certRes)

	addPathToMetadata(meta, domain, certRes, certsStorage)

	return launchHook(ctx.String(flgRenewHook), meta)
}

func renewForCSR(ctx *cli.Context, client *lego.Client, certsStorage *CertificatesStorage, bundle bool, meta map[string]string) error {
	csr, err := readCSRFile(ctx.String(flgCSR))
	if err != nil {
		log.Fatal(err)
	}

	domain, err := certcrypto.GetCSRMainDomain(csr)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	// load the cert resource from files.
	// We store the certificate, private key and metadata in different files
	// as web servers would not be able to work with a combined file.
	certificates, err := certsStorage.ReadCertificate(domain, certExt)
	if err != nil {
		log.Fatalf("Error while loading the certificate for domain %s\n\t%v", domain, err)
	}

	cert := certificates[0]

	var ariRenewalTime *time.Time
	var replacesCertID string

	if !ctx.Bool(flgARIDisable) {
		ariRenewalTime = getARIRenewalTime(ctx, cert, domain, client)
		if ariRenewalTime != nil {
			now := time.Now().UTC()

			// Figure out if we need to sleep before renewing.
			if ariRenewalTime.After(now) {
				log.Infof("[%s] Sleeping %s until renewal time %s", domain, ariRenewalTime.Sub(now), ariRenewalTime)
				time.Sleep(ariRenewalTime.Sub(now))
			}
		}

		replacesCertID, err = certificate.MakeARICertID(cert)
		if err != nil {
			log.Fatalf("Error while construction the ARI CertID for domain %s\n\t%v", domain, err)
		}
	}

	if ariRenewalTime == nil && !needRenewal(cert, domain, ctx.Int(flgDays)) {
		return nil
	}

	// This is just meant to be informal for the user.
	timeLeft := cert.NotAfter.Sub(time.Now().UTC())
	log.Infof("[%s] acme: Trying renewal with %d hours remaining", domain, int(timeLeft.Hours()))

	request := certificate.ObtainForCSRRequest{
		CSR:                            csr,
		NotBefore:                      getTime(ctx, flgNotBefore),
		NotAfter:                       getTime(ctx, flgNotAfter),
		Bundle:                         bundle,
		PreferredChain:                 ctx.String(flgPreferredChain),
		AlwaysDeactivateAuthorizations: ctx.Bool(flgAlwaysDeactivateAuthorizations),
	}

	if replacesCertID != "" {
		request.ReplacesCertID = replacesCertID
	}

	certRes, err := client.Certificate.ObtainForCSR(request)
	if err != nil {
		log.Fatal(err)
	}

	certsStorage.SaveResource(certRes)

	addPathToMetadata(meta, domain, certRes, certsStorage)

	return launchHook(ctx.String(flgRenewHook), meta)
}

func needRenewal(x509Cert *x509.Certificate, domain string, days int) bool {
	if x509Cert.IsCA {
		log.Fatalf("[%s] Certificate bundle starts with a CA certificate", domain)
	}

	if days >= 0 {
		notAfter := int(time.Until(x509Cert.NotAfter).Hours() / 24.0)
		if notAfter > days {
			log.Printf("[%s] The certificate expires in %d days, the number of days defined to perform the renewal is %d: no renewal.",
				domain, notAfter, days)
			return false
		}
	}

	return true
}

// getARIRenewalTime checks if the certificate needs to be renewed using the renewalInfo endpoint.
func getARIRenewalTime(ctx *cli.Context, cert *x509.Certificate, domain string, client *lego.Client) *time.Time {
	if cert.IsCA {
		log.Fatalf("[%s] Certificate bundle starts with a CA certificate", domain)
	}

	renewalInfo, err := client.Certificate.GetRenewalInfo(certificate.RenewalInfoRequest{Cert: cert})
	if err != nil {
		if errors.Is(err, api.ErrNoARI) {
			// The server does not advertise a renewal info endpoint.
			log.Warnf("[%s] acme: %v", domain, err)
			return nil
		}
		log.Warnf("[%s] acme: calling renewal info endpoint: %v", domain, err)
		return nil
	}

	now := time.Now().UTC()
	renewalTime := renewalInfo.ShouldRenewAt(now, ctx.Duration(flgARIWaitToRenewDuration))
	if renewalTime == nil {
		log.Infof("[%s] acme: renewalInfo endpoint indicates that renewal is not needed", domain)
		return nil
	}
	log.Infof("[%s] acme: renewalInfo endpoint indicates that renewal is needed", domain)

	if renewalInfo.ExplanationURL != "" {
		log.Infof("[%s] acme: renewalInfo endpoint provided an explanation: %s", domain, renewalInfo.ExplanationURL)
	}

	return renewalTime
}

func addPathToMetadata(meta map[string]string, domain string, certRes *certificate.Resource, certsStorage *CertificatesStorage) {
	meta[renewEnvCertDomain] = domain
	meta[renewEnvCertPath] = certsStorage.GetFileName(domain, certExt)
	meta[renewEnvCertKeyPath] = certsStorage.GetFileName(domain, keyExt)

	if certRes.IssuerCertificate != nil {
		meta[renewEnvIssuerCertKeyPath] = certsStorage.GetFileName(domain, issuerExt)
	}

	if certsStorage.pem {
		meta[renewEnvCertPEMPath] = certsStorage.GetFileName(domain, pemExt)
	}

	if certsStorage.pfx {
		meta[renewEnvCertPFXPath] = certsStorage.GetFileName(domain, pfxExt)
	}
}

func merge(prevDomains, nextDomains []string) []string {
	for _, next := range nextDomains {
		var found bool
		for _, prev := range prevDomains {
			if prev == next {
				found = true
				break
			}
		}
		if !found {
			prevDomains = append(prevDomains, next)
		}
	}
	return prevDomains
}
