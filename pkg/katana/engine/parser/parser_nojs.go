//go:build windows || 386

package parser

import "katanacrawlgo/pkg/katana/types"

func InitWithOptions(options *types.Options) {
	if options.AutomaticFormFill {
		responseParsers = append(responseParsers, responseParser{bodyParser, bodyFormTagParser})
	}
	if options.ScrapeJSResponses {
		responseParsers = append(responseParsers, responseParser{bodyParser, scriptContentRegexParser})
		responseParsers = append(responseParsers, responseParser{contentParser, scriptJSFileRegexParser})
		responseParsers = append(responseParsers, responseParser{contentParser, bodyScrapeEndpointsParser})
	}
	if !options.DisableRedirects {
		responseParsers = append(responseParsers, responseParser{headerParser, headerLocationParser})
	}
}
