// SPDX-FileCopyrightText: 2016 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/ec2credentials"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/tokens"
	"github.com/sapcc/go-api-declarations/bininfo"
	"github.com/sapcc/go-bits/gophercloudext"
	"github.com/sapcc/go-bits/httpext"
	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/must"
	"github.com/sapcc/go-bits/osext"

	"github.com/sapcc/swift-http-import/pkg/actors"
	"github.com/sapcc/swift-http-import/pkg/objects"
)

func main() {
	startTime := time.Now()

	logg.ShowDebug = osext.GetenvBool("DEBUG")

	// setup a context that shuts down all pipeline actors when an interrupt signal is received
	ctx := httpext.ContextWithSIGINT(context.Background(), 1*time.Second)

	wrap := httpext.WrapTransport(&http.DefaultTransport)
	wrap.SetInsecureSkipVerify(osext.GetenvBool("INSECURE")) // for debugging with mitmproxy etc. (DO NOT SET IN PRODUCTION)
	wrap.SetOverrideUserAgent(bininfo.Component(), bininfo.VersionOr("dev"))

	// read arguments
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: swift-http-import <config-file>")
		fmt.Fprintln(os.Stderr, "   or: swift-http-import --version")
		os.Exit(1)
	}
	if os.Args[1] == "--version" {
		fmt.Println("swift-http-import " + bininfo.VersionOr("dev"))
		os.Exit(0)
	}

	// hidden feature: we use swift-http-import to fill Swift containers
	// which are then exposed via <https://github.com/oxyno-zeta/s3-proxy>;
	// this command is used to issue ec2 credentials for s3-proxy on startup
	// (it does not really need to be in here, but putting it here saves us the trouble of
	// shipping yet another image for what is little more than a single function call)
	//
	// WARNING: This command is not governed by any sort of stability guarantee
	//          and may change or be removed at any point.
	if os.Args[1] == "--replace-and-issue-ec2-credentials" {
		replaceAndIssueEC2Credentials(ctx)
		return
	}

	// read configuration
	config, errs := objects.ReadConfiguration(ctx, os.Args[1])
	if len(errs) > 0 {
		for _, err := range errs {
			logg.Error(err.Error())
		}
		os.Exit(1)
	}

	// setup the Report actor
	reportChan := make(chan actors.ReportEvent)
	report := actors.Report{
		Input:     reportChan,
		Statsd:    config.Statsd,
		StartTime: startTime,
	}
	var wgReport sync.WaitGroup
	actors.Start(ctx, &report, &wgReport)

	// do the work
	runPipeline(ctx, config, reportChan)

	// shutdown Report actor
	close(reportChan)
	wgReport.Wait()
	os.Exit(report.ExitCode)
}

func replaceAndIssueEC2Credentials(ctx context.Context) {
	// get Identity V3 client
	providerClient, eo, err := gophercloudext.NewProviderClient(ctx, nil)
	must.Succeed(err)
	client := must.Return(openstack.NewIdentityV3(providerClient, eo))

	// extract user_id and project_id from auth response
	authResult, ok := providerClient.GetAuthResult().(tokens.CreateResult)
	if !ok {
		logg.Fatal("got unexpected AuthResult type %T", providerClient.GetAuthResult())
	}
	userID := must.Return(authResult.ExtractUser()).ID
	projectID := must.Return(authResult.ExtractProject()).ID

	// delete all previous EC2 credentials for this project scope
	page := must.Return(ec2credentials.List(client, userID).AllPages(ctx))
	for _, cred := range must.Return(ec2credentials.ExtractCredentials(page)) {
		if cred.TenantID == projectID {
			must.Succeed(ec2credentials.Delete(ctx, client, userID, cred.Access).ExtractErr())
		}
	}

	// issue a new EC2 credential and write it into $PWD/{access-key,secret-key}
	opts := ec2credentials.CreateOpts{TenantID: projectID}
	cred := must.Return(ec2credentials.Create(ctx, client, userID, opts).Extract())
	must.Succeed(os.WriteFile("access-key", []byte(cred.Access+"\n"), 0666))
	must.Succeed(os.WriteFile("secret-key", []byte(cred.Secret+"\n"), 0666))
}

func runPipeline(ctx context.Context, config *objects.Configuration, report chan<- actors.ReportEvent) {
	// start the pipeline actors
	var wg sync.WaitGroup
	var wgTransfer sync.WaitGroup
	queue1 := make(chan objects.File, 10)              // will be closed by scraper when it's done
	queue2 := make(chan actors.FileInfoForCleaner, 10) // will be closed by us when all transferors are done

	actors.Start(ctx, &actors.Scraper{
		Jobs:   config.Jobs,
		Output: queue1,
		Report: report,
	}, &wg)

	for range config.WorkerCounts.Transfer {
		actors.Start(ctx, &actors.Transferor{
			Input:  queue1,
			Output: queue2,
			Report: report,
		}, &wg, &wgTransfer)
	}

	actors.Start(ctx, &actors.Cleaner{
		Input:  queue2,
		Report: report,
	}, &wg)

	// wait for transfer phase to finish
	wgTransfer.Wait()
	// signal to cleaner to start its work
	close(queue2)
	// wait for remaining workers to finish
	wg.Wait()
}
