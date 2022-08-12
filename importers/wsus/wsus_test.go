// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wsus

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	storage "google.golang.org/api/storage/v1"
)

var gcsListResponse = `{
    "items": [
        {
            "bucket": "hashr-wsus-test",
            "contentType": "text/plain",
            "crc32c": "AAAAAA==",
            "etag": "CLT8oPmbp+wCEAE=",
            "generation": "1602236461891124",
            "id": "hashr-wsus-test/FF//1602236461891124",
            "kind": "storage#object",
            "md5Hash": "1B2M2Y8AsgTpgAmY7PhCfg==",
            "mediaLink": "https://storage.googleapis.com/download/storage/v1/b/hashr-wsus-test/o/FF%2F?generation=1602236461891124&alt=media",
            "metageneration": "1",
            "name": "FF/",
            "selfLink": "https://www.googleapis.com/storage/v1/b/hashr-wsus-test/o/FF%2F",
            "storageClass": "STANDARD",
            "timeCreated": "2020-10-09T09:41:01.890Z",
            "timeStorageClassUpdated": "2020-10-09T09:41:01.890Z",
            "updated": "2020-10-09T09:41:01.890Z"
        },
        {
            "bucket": "hashr-wsus-test",
            "contentLanguage": "en",
            "contentType": "application/x-cab",
            "crc32c": "J7UkHQ==",
            "etag": "CMPxuZiep+wCEAE=",
            "generation": "1602237064181955",
            "id": "hashr-wsus-test/FF/03E86F3A0947C8A5183AD0C66A48782FA216BEFF.cab/1602237064181955",
            "kind": "storage#object",
            "md5Hash": "Yv7ryTtOzRofzVTkf+We4w==",
            "mediaLink": "https://storage.googleapis.com/download/storage/v1/b/hashr-wsus-test/o/FF%2F03E86F3A0947C8A5183AD0C66A48782FA216BEFF.cab?generation=1602237064181955&alt=media",
            "metageneration": "1",
            "name": "FF/03E86F3A0947C8A5183AD0C66A48782FA216BEFF.cab",
            "selfLink": "https://www.googleapis.com/storage/v1/b/hashr-wsus-test/o/FF%2F03E86F3A0947C8A5183AD0C66A48782FA216BEFF.cab",
            "size": "1615608",
            "storageClass": "STANDARD",
            "timeCreated": "2020-10-09T09:51:04.181Z",
            "timeStorageClassUpdated": "2020-10-09T09:51:04.181Z",
            "updated": "2020-10-09T09:51:04.181Z"
        },
        {
            "bucket": "hashr-wsus-test",
            "contentLanguage": "en",
            "contentType": "application/x-cab",
            "crc32c": "t+7YVQ==",
            "etag": "CLva/Ziep+wCEAE=",
            "generation": "1602237065293115",
            "id": "hashr-wsus-test/FF/138ECA2DEB45E284DC0BB94CC8849D1933B072FF.cab/1602237065293115",
            "kind": "storage#object",
            "md5Hash": "kCgm6/6LnYeS1qaIiEjg4Q==",
            "mediaLink": "https://storage.googleapis.com/download/storage/v1/b/hashr-wsus-test/o/FF%2F138ECA2DEB45E284DC0BB94CC8849D1933B072FF.cab?generation=1602237065293115&alt=media",
            "metageneration": "1",
            "name": "FF/138ECA2DEB45E284DC0BB94CC8849D1933B072FF.cab",
            "selfLink": "https://www.googleapis.com/storage/v1/b/hashr-wsus-test/o/FF%2F138ECA2DEB45E284DC0BB94CC8849D1933B072FF.cab",
            "size": "4106633",
            "storageClass": "STANDARD",
            "timeCreated": "2020-10-09T09:51:05.292Z",
            "timeStorageClassUpdated": "2020-10-09T09:51:05.292Z",
            "updated": "2020-10-09T09:51:05.292Z"
        },
        {
            "bucket": "hashr-wsus-test",
            "contentLanguage": "en",
            "contentType": "application/x-cab",
            "crc32c": "rndI9g==",
            "etag": "CNaQoJmep+wCEAE=",
            "generation": "1602237065857110",
            "id": "hashr-wsus-test/FF/1BDBDA1C53B6C980DD440B93646D8021CC90F1FF.cab/1602237065857110",
            "kind": "storage#object",
            "md5Hash": "WBAjVtsdW4zBI6dRk5Schw==",
            "mediaLink": "https://storage.googleapis.com/download/storage/v1/b/hashr-wsus-test/o/FF%2F1BDBDA1C53B6C980DD440B93646D8021CC90F1FF.cab?generation=1602237065857110&alt=media",
            "metageneration": "1",
            "name": "FF/1BDBDA1C53B6C980DD440B93646D8021CC90F1FF.cab",
            "selfLink": "https://www.googleapis.com/storage/v1/b/hashr-wsus-test/o/FF%2F1BDBDA1C53B6C980DD440B93646D8021CC90F1FF.cab",
            "size": "1588616",
            "storageClass": "STANDARD",
            "timeCreated": "2020-10-09T09:51:05.856Z",
            "timeStorageClassUpdated": "2020-10-09T09:51:05.856Z",
            "updated": "2020-10-09T09:51:05.856Z"
        },
				{
            "bucket": "hashr-wsus-test",
            "contentLanguage": "en",
            "contentType": "application/x-msdos-program",
            "crc32c": "AAAAAA==",
            "etag": "CKuZ5Zmep+wCEAE=",
            "generation": "1602237066988715",
            "id": "hashr-wsus-test/FF/Update.exe/1602237066988715",
            "kind": "storage#object",
            "md5Hash": "1B2M2Y8AsgTpgAmY7PhCfg==",
            "mediaLink": "https://storage.googleapis.com/download/storage/v1/b/hashr-wsus-test/o/FF%2FUpdate.exe?generation=1602237066988715&alt=media",
            "metageneration": "1",
            "name": "FF/Update.exe",
            "selfLink": "https://www.googleapis.com/storage/v1/b/hashr-wsus-test/o/FF%2FUpdate.exe",
            "storageClass": "STANDARD",
            "timeCreated": "2020-10-09T09:51:06.988Z",
            "timeStorageClassUpdated": "2020-10-09T09:51:06.988Z",
            "updated": "2020-10-09T09:51:06.988Z"
        },
				{
            "bucket": "hashr-wsus-test",
            "contentLanguage": "en",
            "contentType": "application/x-cab",
            "crc32c": "5NfFZA==",
            "etag": "CIWVvZmep+wCEAE=",
            "generation": "1602237066332805",
            "id": "hashr-wsus-test/FF/1F35F72D34C16FF7D7270D60472D8AD9FF9D7EFF.cab/1602237066332805",
            "kind": "storage#object",
            "md5Hash": "I7YxrYi3Ky6ZOUi6niywNQ==",
            "mediaLink": "https://storage.googleapis.com/download/storage/v1/b/hashr-wsus-test/o/FF%2F1F35F72D34C16FF7D7270D60472D8AD9FF9D7EFF.cab?generation=1602237066332805&alt=media",
            "metageneration": "1",
            "name": "FF/1F35F72D34C16FF7D7270D60472D8AD9FF9D7EFF.cab",
            "selfLink": "https://www.googleapis.com/storage/v1/b/hashr-wsus-test/o/FF%2F1F35F72D34C16FF7D7270D60472D8AD9FF9D7EFF.cab",
            "size": "951153",
            "storageClass": "STANDARD",
            "timeCreated": "2020-10-09T09:51:06.332Z",
            "timeStorageClassUpdated": "2020-10-09T09:51:06.332Z",
            "updated": "2020-10-09T09:51:06.332Z"
        },
        {
            "bucket": "hashr-wsus-test",
            "contentLanguage": "en",
            "contentType": "text/plain",
            "crc32c": "AAAAAA==",
            "etag": "CPi105mep+wCEAE=",
            "generation": "1602237066697464",
            "id": "hashr-wsus-test/FF/another_file.txt/1602237066697464",
            "kind": "storage#object",
            "md5Hash": "1B2M2Y8AsgTpgAmY7PhCfg==",
            "mediaLink": "https://storage.googleapis.com/download/storage/v1/b/hashr-wsus-test/o/FF%2Fanother_file.txt?generation=1602237066697464&alt=media",
            "metageneration": "1",
            "name": "FF/another_file.txt",
            "selfLink": "https://www.googleapis.com/storage/v1/b/hashr-wsus-test/o/FF%2Fanother_file.txt",
            "storageClass": "STANDARD",
            "timeCreated": "2020-10-09T09:51:06.697Z",
            "timeStorageClassUpdated": "2020-10-09T09:51:06.697Z",
            "updated": "2020-10-09T09:51:06.697Z"
        }
    ],
    "kind": "storage#objects"
}`

type mockTransport struct {
	responses []mockResponses
	index     int
}

// RoundTrip is an http.Client.Transport implementation. This has to be exported to satisfy
// http.RoundTripper interface.
func (c *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	if c.index >= len(c.responses) {
		resp = &http.Response{
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			StatusCode: 503,
			Status: fmt.Sprintf("Got more requests than mocked Response. Index: %d, Response: %d",
				c.index, len(c.responses)),
			Header:  make(http.Header),
			Request: req,
		}
	} else {
		header := make(http.Header)
		for k, v := range c.responses[c.index].Header {
			header.Add(k, v)
		}
		body := ioutil.NopCloser(strings.NewReader(c.responses[c.index].Body))
		resp = &http.Response{
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			StatusCode: c.responses[c.index].StatusCode,
			Body:       body,
			Header:     header,
			Request:    req,
		}
	}
	c.index++
	return resp, nil
}

func (c *mockTransport) add(StatusCode int, Body string, Header map[string]string) {
	c.responses = append(c.responses, mockResponses{StatusCode, Header, Body})
}

type mockResponses struct {
	StatusCode int
	Header     map[string]string
	Body       string
}

func mockHTTPClientAndTransport() (*http.Client, *mockTransport) {
	mockTransport := mockTransport{make([]mockResponses, 0), 0}
	httpClient := &http.Client{}
	httpClient.Transport = &mockTransport
	return httpClient, &mockTransport
}

func TestDiscoverRepo(t *testing.T) {
	ctx := context.Background()
	repo := &Repo{}

	storageHTTPClient, storageHTTPTransport := mockHTTPClientAndTransport()
	storageHTTPTransport.add(404, "", nil)
	storageHTTPTransport.add(200, gcsListResponse, nil)

	var err error
	storageClient, err = storage.NewService(ctx, option.WithHTTPClient(storageHTTPClient))
	if err != nil {
		t.Fatalf("could not create mock GCE client: %v", err)
	}

	gotSources, err := repo.DiscoverRepo()
	if err != nil {
		t.Fatalf("unexpected error while running DiscoverRepo(): %v", err)
	}

	wantUpdates := []*update{
		{
			id:         "03E86F3A0947C8A5183AD0C66A48782FA216BEFF",
			remotePath: "FF/03E86F3A0947C8A5183AD0C66A48782FA216BEFF.cab",
			md5hash:    "Yv7ryTtOzRofzVTkf+We4w==",
			format:     cabArchive,
		},
		{
			id:         "138ECA2DEB45E284DC0BB94CC8849D1933B072FF",
			remotePath: "FF/138ECA2DEB45E284DC0BB94CC8849D1933B072FF.cab",
			md5hash:    "kCgm6/6LnYeS1qaIiEjg4Q==",
			format:     cabArchive,
		},
		{
			id:         "1BDBDA1C53B6C980DD440B93646D8021CC90F1FF",
			remotePath: "FF/1BDBDA1C53B6C980DD440B93646D8021CC90F1FF.cab",
			md5hash:    "WBAjVtsdW4zBI6dRk5Schw==",
			format:     cabArchive,
		},
		{
			id:         "Update",
			remotePath: "FF/Update.exe",
			md5hash:    "1B2M2Y8AsgTpgAmY7PhCfg==",
			format:     exe,
		},
		{
			id:         "1F35F72D34C16FF7D7270D60472D8AD9FF9D7EFF",
			remotePath: "FF/1F35F72D34C16FF7D7270D60472D8AD9FF9D7EFF.cab",
			md5hash:    "I7YxrYi3Ky6ZOUi6niywNQ==",
			format:     cabArchive,
		},
	}

	var gotUpdates []*update
	for _, source := range gotSources {
		if image, ok := source.(*update); ok {
			gotUpdates = append(gotUpdates, image)
		} else {
			t.Fatal("error while casting Source interface to update struct")
		}
	}

	if !cmp.Equal(wantUpdates, gotUpdates, cmp.AllowUnexported(update{})) {
		t.Errorf("DiscoverRepo() unexpected diff (-want/+got):\n%s", cmp.Diff(wantUpdates, gotUpdates, cmp.AllowUnexported(update{})))
	}
}

func TestPreprocess(t *testing.T) {
	ctx := context.Background()
	type extraction struct {
		update string
		files  map[string]string
	}

	for _, tc := range []struct {
		update         *update
		testdataPath   string
		wantExtraction extraction
	}{
		{
			update: &update{
				id:         "03E86F3A0947C8A5183AD0C66A48782FA216BEFF",
				remotePath: "FF/03E86F3A0947C8A5183AD0C66A48782FA216BEFF.cab",
				format:     cabArchive,
			},
			testdataPath: "testdata/1BDBDA1C53B6C980DD440B93646D8021CC90F1FF.cab",
			wantExtraction: extraction{
				update: "03E86F3A0947C8A5183AD0C66A48782FA216BEFF",
				files: map[string]string{
					"!Binary":                 "304055b8d7c8561a27d454a206dfcfb68bbecf943ad38bf961534e7cc386acbb",
					"!CustomAction":           "5df15a83e054969a86399c41be76f107f21d0baf16db5b3f4bb96ea598605724",
					"!File":                   "20d4f1e3a22ad2d2f5911173ae5934637b427cf7c52fabfb61e97b1058ec241b",
					"!InstallExecuteSequence": "9340dcca4e602f2622a4e0ade9c503ff8e9743d7d735ccce1b74be90990b3ad7",
					"!Media":                  "6d83c37882bbfab0ab73cdd2c895f1c011eb347c06ab28e25d4408fb745c4545",
					"!MsiAssemblyName":        "6a5533e36f1014ef19dd188e5f494954e45a1ba35b812f69a8d2899c3b6c3fad",
					"!MsiFileHash":            "a8016c9ecfcb586452cd0be7f0ad20d8bd4dd5bd601ee66a21d89346a6148a2c",
					"!MsiPatchMetadata":       "24ae514376bd0e198f88b7f88d181c16aea8ef5f5ddfd97cc329347c5ed01e4c",
					"!MsiPatchSequence":       "a2c6f0b861db0cb40d8a5b76f1b9c7f3a4489f7d0b0567fa6b72d8d15e7c09e5",
					"!PatchPackage":           "bec77fea1415d62adef633a6b44cd59937f2d4d73f13d326a9c658f5e61de32f",
					"!Property":               "97e3a0953e196a2d0cb643a2438afefa58fce558da7a520a7b933f484ca9405e",
					"!_Columns":               "a81db5ce42036ae1a4255f3b6ef4db8a153d7631fe766ae9d6a85bb905aa7441",
					"!_StringData":            "1735b61d6118f103921ba4cd02257fa106b35db10ae7ce3a447357aa4a0496dd",
					"!_StringPool":            "d7b95a801ffd77dd1b8b3faf18931dccdc5eef24cf50b115d020f6daf81fc4af",
					"!_Tables":                "f896c3a5f9841b6e1f0a22bd35a6a1bc5efb28aaa23b66301ec8098ce57cf99a",
					"Binary.AbortMsiCA.dll":   "29bb2e649faf13b2a92b69315cb45820caabadc39c51871569bea25b380db6e2",
					"Binary.CADLL":            "aeb905ef34d9ffa2789932602dc30927736a6ea43ead19054cc4e6cfd589a02f",
					"Binary.OCFXCA":           "23af907c448f8caf818292321b9eeb813f7a515df1889987d81431fba2edfe9e",
					"Binary.patchca.dll":      "bb1715da0adcd84e0f8ac26a18ad5fc828fdb1fd45f48b63d62b1a3385ed80ee",
					"PATCH_CAB":               "6542f9b4531774ed49765e28435d0dbff6a1a6abd39a554268d4ff5ac4fe67c9",
					"[5]SummaryInformation":   "046c3148641aa305024667c56b160c0b5c0a75395bff09222e4f5af793134208",
					"wssmui-ms-my.msp":        "4c7470ea18b2bd8fe5a74a54014f2c8039dc91ba0560e5d67868c114ab4d23a7",
					"wssmui-ms-my.xml":        "0e0bd37418bcdcd6663f7a1ad5a335f180a296b3282d834e574a387203fbd560",
					"ADDGALLERY.GLOBALIZATION.RESOURCES.DLL_1086": "3fc3b41cd47a2ef72f6f847650a61093374712fd15c7e15300c77de89b4919a1",
					"BFORM.DEBUG.JS_1086":                         "8681c9467b0677f6e64bb501dbc4b675cfe180002a273ee65d7c30297ba479b1",
					"BFORM.JS_1086":                               "6584f8fac632c0b0b2c10550090adf03a2fbca620586c38f7aa9b8e73d531c46",
					"BLOG.CSS_1086":                               "4687ed6ff903569172299d8d612591529478450e041f6321c56a2f395cd9a16f",
					"BLOG_V4T.CSS_1086":                           "4687ed6ff903569172299d8d612591529478450e041f6321c56a2f395cd9a16f",
					"BPSTD.ASX_1086":                              "ad1e7b8ec95cd0770b7b3c3afdca72b38ec6d74d4f6fcd814f782ad6a933bf5f",
					"CALENDAR.CSS_1086":                           "1bb916989a62e3a7f52826fe6c6257fef015af939c4182f3a23bae38ce2782e3",
					"CALENDARV4.CSS_1086":                         "437efe93ab4684a952bece0eb0e2356cb7e14453752b817e5525af88fb383a40",
					"CALENDARV4_V4T.CSS_1086":                     "437efe93ab4684a952bece0eb0e2356cb7e14453752b817e5525af88fb383a40",
					"CALENDAR_V4T.CSS_1086":                       "1bb916989a62e3a7f52826fe6c6257fef015af939c4182f3a23bae38ce2782e3",
					"CORE.CSS_1086":                               "8c8d91619f4fea5eeb26f675fd03d753df89b8b6dd198157208e5f8886e756fa",
					"CORE.DEBUG.JS_1086":                          "158bf5e2250c20563ebded0c45ff7cae3110fa89b622c814231f5a8a5e603b70",
					"CORE.JS_0001_1086":                           "341013dd057ecf38022f5640d448b8e2d9e1cd62fcd86690d79214550e4c8b5f",
					"CORE.RSX_1086":                               "c51afeaada79e9f606cf5077d8a832131318ad835cbd06568e540777924b20ff",
					"COREV4.CSS_1086":                             "d2cb8bd4cb68c594b26a99d5020eb4faf0cac580edf2e7f4284af358edf0643d",
					"COREV4_V4T.CSS_1086":                         "d2cb8bd4cb68c594b26a99d5020eb4faf0cac580edf2e7f4284af358edf0643d",
					"CUI.CSS_1086":                                "c1ee3e46828b07953daddf9d1446e457824a5a9a6f017b5f73cd347ba108fa73",
					"CUIDARK.CSS_1086":                            "df681deec2d54670931c995e95333794c1fedfcfc620d877f2cefe9029522946",
					"CUIDARK_V4T.CSS_1086":                        "df681deec2d54670931c995e95333794c1fedfcfc620d877f2cefe9029522946",
					"CUI_V4T.CSS_1086":                            "c1ee3e46828b07953daddf9d1446e457824a5a9a6f017b5f73cd347ba108fa73",
					"DATEPICK.CSS_1086":                           "aa0744131484d791c83f94ab4e8e7f26ef8b20e3e20d5b5663ce329aae91f046",
					"DATEPICK_V4T.CSS_1086":                       "a8f20dce1345fe2ce88c62eef24bc4063263055a75d432b47ed495c741dba2d0",
					"DATEPKV4.CSS_1086":                           "a8f20dce1345fe2ce88c62eef24bc4063263055a75d432b47ed495c741dba2d0",
					"DISCTHRD.CSS_1086":                           "d89baf1b812dfc6b383430e7e8763645b6310436993c987acc0fbf2dc9853edc",
					"DISCTHRD_V4T.CSS_1086":                       "d89baf1b812dfc6b383430e7e8763645b6310436993c987acc0fbf2dc9853edc",
					"ERRORV4.HTM_1086":                            "9710af2b40d34f2e8ae055594b766f5874b5ab8b5788910c0550575cc8568b6b",
					"FORM.DEBUG.JS_1086":                          "1f3097cc010c99a4c511bde1ec028e01d5d1d506eb3871e820d3ee477a85d0fa",
					"FORM.JS_1086":                                "90fbe763db68de4f761f829b9fa9d88527cf441e7ae508f0bac18d94e8670606",
					"FORMS.CSS_1086":                              "d0679b67a54c21e7237489c4d2a292666a022f6d9b44efacafb3b539bb0ee3cc",
					"FORMS_V4T.CSS_1086":                          "d0679b67a54c21e7237489c4d2a292666a022f6d9b44efacafb3b539bb0ee3cc",
					"GANTTWSS.CSS_1086":                           "2fe546ea53525051e44c411738d09ca73fcaaa9def5affa6ac08103530653a64",
					"GROUPBOARD.CSS_1086":                         "9fac742363aff5239f4f9becf7fd251af1dc499f4eadc9eb899a34c9c4d358bf",
					"GROUPBOARD_V4T.CSS_1086":                     "9fac742363aff5239f4f9becf7fd251af1dc499f4eadc9eb899a34c9c4d358bf",
					"HELP.CSS_1086":                               "712e9589a860dfd075ac53ae52c029013b477040e08a1cc9b723b292a8d906f9",
					"HELP_V4T.CSS_1086":                           "712e9589a860dfd075ac53ae52c029013b477040e08a1cc9b723b292a8d906f9",
					"INIT.DEBUG.JS_1086":                          "453f88b64e69aa9d65d80eec06249a6a99dcd91984d0270451175d030004dc77",
					"INIT.JS_0001_1086":                           "4a300d10078373b49ac49acf6b0acefc2dd10a93a4ee97f4f1c2991312a5d685",
					"LAYOUTS.CSS_1086":                            "d5c2adfb05ec73c444941be90d37f5ea4cdb0786e248b09f322c5d0065716ad2",
					"LAYOUTS_V4T.CSS_1086":                        "d5c2adfb05ec73c444941be90d37f5ea4cdb0786e248b09f322c5d0065716ad2",
					"MBLRTE.CSS_1086":                             "46530ff74eb7f936189a160d5b08ef630d12ede51f0e9693ce171c5fdf4b4eac",
					"MBLRTE_V4T.CSS_1086":                         "46530ff74eb7f936189a160d5b08ef630d12ede51f0e9693ce171c5fdf4b4eac",
					"MENU.CSS_1086":                               "357bab503d85f83bd557a361c762b6a530db0bb916531c41ed2daaa5b27e4794",
					"MENU21.CSS_1086":                             "ad9bda15aad6767a7c670a6b891c154196b55a0e2899d19eb8129d67bad884fe",
					"MENU_V4T.CSS_1086":                           "357bab503d85f83bd557a361c762b6a530db0bb916531c41ed2daaa5b27e4794",
					"MINIMALV4.CSS_1086":                          "5ba9a43389e4ffc9c8e7336fbcc75a9643b894e92276706c6bc31fc725dcd6f7",
					"MINIMALV4_V4T.CSS_1086":                      "5ba9a43389e4ffc9c8e7336fbcc75a9643b894e92276706c6bc31fc725dcd6f7",
					"MS.SP.PS.INTL.RESOURCES.DLL_1086":            "ebfcc24ae830deb9c42dd277338efe891378a7e06eb2df4cee28ef62fecace9b",
					"MWS.CSS_1086":                                "5096aa5e94e90e4b45119e1a822faa1a2a85287e638567a52783e017c818bad0",
					"MWS_V4T.CSS_1086":                            "5096aa5e94e90e4b45119e1a822faa1a2a85287e638567a52783e017c818bad0",
					"OWS.DEBUG.JS_1086":                           "d371b932526b062976a5864d9f0f3785bcaab36fedf1a6477c57780939dfb97a",
					"OWS.JS_1086":                                 "45649f7464b5561ca24a76ec89afe6ec0edf3681fea63855d33c553fbd9f7ba2",
					"OWSBROWS.DEBUG.JS_1086":                      "67e5d3634a630063db447625ed4f24a5dea5b39e1cfa62bae7aee9b3ce605947",
					"OWSBROWS.JS_1086":                            "3597ef26f35e4eb9a20bfd370d1d3800ffed79718c47799c83b2c7a1e7325e40",
					"OWSNOCR.CSS_1086":                            "d494d197baacdffc430610d409e6ec30594d6fcc1fd83ac6e95b5af46073682f",
					"OWSNOCR_V4T.CSS_1086":                        "d494d197baacdffc430610d409e6ec30594d6fcc1fd83ac6e95b5af46073682f",
					"PICKERTREE.CSS_1086":                         "09d95927021d59649deb1b6d998963e289e9cb7d8b6beb84e537a3286cb86a96",
					"RGNLSTNG.XML_1086":                           "cf7a5913856f57302cab8070b3b764d4f5d9ac512b9a59d7004567281f69f20d",
					"SPMSGP.DLL_1086":                             "2679f0c75c057d79fe7c5b9b9d3307f1702bd8136168788f41b8e111be36bade",
					"SPSTD1.ASX_0001_1086":                        "a3ac1f9c7604230f8732bf2f568c2deb04b9649781a9b077ce311ba7e78b12aa",
					"SPSTD2.ASX_0001_1086":                        "88400b3c0ecb94928a70a4fbb28061ad0567aa678cfe68f6cf2ed2b1c8aaf4e8",
					"SPSTD3.ASX_1086":                             "8ca2a67321e7ab6e45b582a718d3c91308ad0e2aa4fbcc732f4e9f5de983c7c8",
					"SPSTD4.ASX_1086":                             "4b3cae47a9e6d256d19bfd6d626a326d8ebb15291c10a0a5ce9283afa71271ba",
					"SPSTD5.ASX_1086":                             "3662498cf1a04701f35243e633592d316545d7e69bb8914b41171d6cd2289998",
					"SPSTD6.ASX_1086":                             "76c8f49a6b25a1b8cf24b54c3feb27b65502f42c42b07ef1426be158137bbae8",
					"SPSTD7.ASX_1086":                             "57f8c4eddbcc4dc9b38b7f449b628c66ff6137b92737f35358324fcab4b31440",
					"SPSTD8.ASX_1086":                             "cc6115c30de81e14719d6dcb6b466e81737b132d03c955476ae84277cb5a0eb0",
					"STSOMR.DLL_1086":                             "08d1d491be4f264a76a196098a95b8ea57d83357591eacba9f5e8b6dafffa11b",
					"SURVEY.CSS_1086":                             "2951296af2bdfbc99753e53cf869a25df2a4611d581d1101c10c64eb5676d051",
					"SURVEY_V4T.CSS_1086":                         "2951296af2bdfbc99753e53cf869a25df2a4611d581d1101c10c64eb5676d051",
					"THEMEV4_V4T.CSS_1086":                        "02193adc17d54b88c5ac075b563e19c2015ee8631a9cdf04f5f0759dcd07de68",
					"WIKI.CSS_1086":                               "4e47b5f0edc6f5db720238ad61e99ea8d3de74caa7299552c07bf60d3ff9de9d",
					"WIKI_V4T.CSS_1086":                           "4e47b5f0edc6f5db720238ad61e99ea8d3de74caa7299552c07bf60d3ff9de9d",
					"WPEDMDV4.CSS_1086":                           "78bb7628d1e4e788b1540c567c5d2f101dd0c026718595534fcbefa61ffde8d6",
					"WPEDMDV4_V4T.CSS_1086":                       "78bb7628d1e4e788b1540c567c5d2f101dd0c026718595534fcbefa61ffde8d6",
					"WPEDTMDE.CSS_1086":                           "8752be93a9bdb612b62b7d0474aa089d775c649943c80db01ed1b86c9a5db87a",
					"WSS.INTL.RES.DLL.1086":                       "74e883fa84afaf7a3c91e83dabfdea0c69fc4b1cc30922dd95cf60a9b671126b",
					"WSSLCID.RSX_1086":                            "00903ae2d6de754ab20cf789087a89a7d15dca0a9f2d5e2b120afb4318eb7fa2",
				},
			},
		},

		{
			update: &update{
				id:         "1F35F72D34C16FF7D7270D60472D8AD9FF9D7EFF",
				remotePath: "FF/1F35F72D34C16FF7D7270D60472D8AD9FF9D7EFF.cab",
				format:     cabArchive,
			},
			testdataPath: "testdata/1F35F72D34C16FF7D7270D60472D8AD9FF9D7EFF.cab",
			wantExtraction: extraction{
				update: "1F35F72D34C16FF7D7270D60472D8AD9FF9D7EFF",
				files: map[string]string{
					"0":                  "e6ad1dc75c73060262d5e22bdfd898ee6fadb6dca6aa184857cdebf25ce68199",
					"1":                  "54e807fda795fb7a68780da63164d4e9257345fdb293b18a26f77039b8d17b9c",
					"_manifest_.cix.xml": "be6aef50bcb0fc923b8fbd1acf73f85403b061afa147babf446d1cc1a10cb7ba",
					"amd64_5ede86eee6f0964dfa08a432ba9ce277_31bf3856ad364e35_6.3.9600.17664_none_0a37e4d3b616094d.manifest":         "56ea9e17c15399eb3ed6acb27c80f9e5855a0bc609a0b02a2af93d081183c5c0",
					"amd64_a53d8df0974951d28215d083b2a7cfcb_31bf3856ad364e35_6.3.9600.17664_none_803904d623084bf0.manifest":         "1f75d250c6fb11e1557b4d78b476a0feba6e5d2deea47517091d8032a7f92bd9",
					"amd64_microsoft-windows-t..icesframework-msctf_31bf3856ad364e35_6.3.9600.17664_none_66979ee6c2abf941.manifest": "a5ae8269dd551b8a2ccce22ccfe4ed4b74c1a17a5aa11b3fe1c9e02d1693a386",
					"package_1_for_kb3033889~31bf3856ad364e35~amd64~~6.3.1.0.cat":                                                   "82db5fa178d43046f1e82da4594fdcd40bc6784b15aab6fe762cdca183d0a6b6",
					"package_1_for_kb3033889~31bf3856ad364e35~amd64~~6.3.1.0.mum":                                                   "23c65159968e80c20023e67a767f4529242d91066d75ae4cc014178e3c756ed0",
					"package_2_for_kb3033889~31bf3856ad364e35~amd64~~6.3.1.0.cat":                                                   "8e03a3ef142224497c95c7391b3f1705e4cd5e265d69cdf368d9a536de435b8e",
					"package_2_for_kb3033889~31bf3856ad364e35~amd64~~6.3.1.0.mum":                                                   "50f2c60462fe76a582becadf71379d7893457ebbc3c7f40a025857eb5fe640ac",
					"package_3_for_kb3033889~31bf3856ad364e35~amd64~~6.3.1.0.cat":                                                   "9f191c4ea7227d7b3efae4a33c1ff1cd6df261661eef3fb7a0c8543fb1c82875",
					"package_3_for_kb3033889~31bf3856ad364e35~amd64~~6.3.1.0.mum":                                                   "f6219a763a16f4020e23f6ff8742b3aee51fcbf416a52ad2d6043ad889fc5abc",
					"package_for_kb3033889_rtm_gm~31bf3856ad364e35~amd64~~6.3.1.0.cat":                                              "3d6cededcc058533728726fa3e02d9d0ed35f0e7236851ae009d3ea8ccd04ee2",
					"package_for_kb3033889_rtm_gm~31bf3856ad364e35~amd64~~6.3.1.0.mum":                                              "965c607d6a5ab33e25210bd553c5969ce62d44d99c1df0ff66d2e93cff111ac9",
					"package_for_kb3033889_rtm~31bf3856ad364e35~amd64~~6.3.1.0.cat":                                                 "a63bd6703600c39a8272a1d5e1123a3c0b3b244cc400482b40f617b9a7b6d695",
					"package_for_kb3033889_rtm~31bf3856ad364e35~amd64~~6.3.1.0.mum":                                                 "9dfd016c01a0ea5d3aa9baa0ada410570335dd31831d5b5c357c93d2760f7fe8",
					"update.cat": "f44c9914bac1ae4fcfeb2566c9f50d8f5d718b083a2bf0710c4a20cd3ead0e82",
					"update.mum": "4263026a720ae4710b39979077891280e5600d9ffbb1d55966a9ce8523db2bf4",
					"x86_microsoft-windows-t..icesframework-msctf_31bf3856ad364e35_6.3.9600.17664_none_0a7903630a4e880b.manifest": "9bb3006d355007a57b929838d6bcb5bf13c6169e5df93b1aecb6c99a499c7146",
				},
			},
		},

		{
			update: &update{
				id:         "03E86F3A0947C8A5183AD0C66A48782FA216BEFF",
				remotePath: "03E86F3A0947C8A5183AD0C66A48782FA216BEFF.cab",
				format:     cabArchive,
			},
			testdataPath: "testdata/03E86F3A0947C8A5183AD0C66A48782FA216BEFF.cab",
			wantExtraction: extraction{
				update: "03E86F3A0947C8A5183AD0C66A48782FA216BEFF",
				files: map[string]string{
					"!Binary":                 "4cc4bfee57e749311862da4c605737b3070756adad14f1a9cbe8198299a1f3aa",
					"!CustomAction":           "1eee345f01109828bd9896082f7635143fe3992c5eee6ec97c0172419e86ff39",
					"!Directory":              "f649b0ffc265bdb9c08717a09a41f2d4d266752d41be62a575a19c03974664d7",
					"!File":                   "2bb6d8b41f15c45fcc7be2fc83fe50d248d2eb3b0050e4bbe9069537f35ff501",
					"!InstallExecuteSequence": "8e791f7b5c02b4898712e7971f843430c2dc563078631da1228208d17518b964",
					"!Media":                  "b9498f69ea5ac5e82d65b95ff3361ba2157d464e4dc753e7958fde631da8c03f",
					"!MsiAssemblyName":        "dc4dc687f1b0be6221030a9deba8e5870460496ddf55fcdb3a6c3dd456aceb0f",
					"!MsiFileHash":            "b44c53261fbc7bc1c3d07a0ef3381d4cf58f77119e61b019b2ebf052513b2c09",
					"!MsiPatchMetadata":       "9f1ea284df5aa89c5b591f07a647c915bd73f527daf3486e6b985ee3422bb560",
					"!MsiPatchSequence":       "584ad6bf2321a0c2ffa374c7ca3a4513fa64f15de2f00ba1ae35bdd6c9d68fa5",
					"!PatchPackage":           "a3720f2103b9a8660e45c2ae13ed7f201ad3f05ee86a691a8c6f60b6ac75dc16",
					"!Property":               "9c5bc754bae6f4fa2abef0b47d40354a32cfa35ffa7ef8c0d95c348137a5f862",
					"!_Columns":               "a81db5ce42036ae1a4255f3b6ef4db8a153d7631fe766ae9d6a85bb905aa7441",
					"!_StringData":            "02e3f53f81a59274fda4f7a40e7d41dcf9e8b9740c9e66d55528997ca5f3524b",
					"!_StringPool":            "bfe8198ff63296cdeeb982af2cc2a4221ba9c578bc448fda24d2da51bb593c20",
					"!_Tables":                "f896c3a5f9841b6e1f0a22bd35a6a1bc5efb28aaa23b66301ec8098ce57cf99a",
					"Binary.AbortMsiCA.dll":   "0b156bc9c416bc0fab29e9d35d80cf4f9b01c9bbe4eab69c1bf64fce117ffb82",
					"Binary.CADLL":            "253d2e979f280304995ca5e954c68b8fd47f9afe6848c197694292b718784867",
					"Binary.OCFXCA":           "391fd7f2b7c9dc3677c9ef9b0667ca50d5069b8bf13cdaecfd5a0f84c555328f",
					"Binary.patchca.dll":      "81f8bf7316893ce5a179378018393d26a54c4bd7963b334cb3366bbf1fa3f572",
					"Binary.rmaddlocalca.dll": "2149edd254c18cfc8a8856d5d1acdd2a614e75b42649abf55aaf8bd1adc5c433",
					"PATCH_CAB":               "501f19094aecad8f5289e91c6dfbef0c25b8c3933346340a49cb63e65de2e7fd",
					"[5]SummaryInformation":   "25bc53dd3c4fb46f21d6d682739ecc2b013a602bdde7bf6872723eacc83b81c1",
					"xlsrvmui-ms-my.msp":      "bd26e746fb8a18acb0a2c6121dfc8c2e53f7a4fc6d39ad124a1885a26494cb14",
					"xlsrvmui-ms-my.xml":      "e393c7c85bba560f3983e132517677d85a8b5ca0a775fc9eac433bac861947d2",
					"ADOMDCLIENT.RES_1086":    "f95ddcca13e703af5d2424dcccf3a6e3b4e1341e68bdbb094bf59f3e12e84a42",
					"AMO.RES_1086":            "3dc2c23f536597e82edd39ede448cd5ff3560e0e1df53025928ae0da9fdfaf11",
					"AS_ClientMsmdsrv_rll_32_1086.4510CDFF_ACD2_4468_8145_5527F9ACF130":  "ab84f0e29a987db666aee2a33290da00d7256a2735531f90fae7111e684a75ed",
					"AS_ClientMsmdsrv_rll_64_1086.4510CDFF_ACD2_4468_8145_5527F9ACF130":  "e2573793be5e574c418283a828d1a733a246a91e6c13e34d2a176d3516117add",
					"AS_ClientMsmdsrvi_rll_32_1086.4510CDFF_ACD2_4468_8145_5527F9ACF130": "4b8b0980c5341ae8b5416b184eca85acd51075c2a2c2d979c844c12067e98dd0",
					"AS_ClientMsmdsrvi_rll_64_1086.4510CDFF_ACD2_4468_8145_5527F9ACF130": "e0ba6d2e2feb118000d13b2138831c87aa62aad6329881a5438b5e6f82c76bc7",
					"AS_msolui110_rll_32_1086.4510CDFF_ACD2_4468_8145_5527F9ACF130":      "9d72736494b5226227b54c6f9371c368d63e680097f85f7ea72842adf01a15fd",
					"AS_msolui110_rll_64_1086.4510CDFF_ACD2_4468_8145_5527F9ACF130":      "a97375f257962772a232b5138ea07f51d59d94773c0ca33f96cafa67bb133e71",
					"BIEXPLORE.CSS.1086":                     "bf85da996e873eb37c9a47cc67e9bb6022c147042356cc0c39372522bbda8338",
					"BISERVER.INTL.COMMONINTL.JS.1086":       "3165e2eeaaca351f97eb4e8f08f3059f0f421d06f5dcf1ae753e26f650025d90",
					"BISERVER.INTL.EWA.READTOOLBAR.JS.1086":  "432c0678fd533e6b6e409a25a0e1ad64c2975c386f5736a6adf06564ed9dc21e",
					"BISERVER.INTL.EWA.RIBBON.JS.1086":       "acc9c443c8dfa96824f0cc129fde32e7531740c9edeea5a8cde1670e1a3ac58a",
					"BISERVER.INTL.EWA.STATUSBAR.JS.1086":    "7c0634c5cc2e2a616730acdd56be35f6f5aa7a316038baa1a3e19e796ca88246",
					"BISERVER.INTL.EWA.STRINGS.MOSS.JS.1086": "95cf3071b41088e74361a90e7057295e915cbc6115d39676d4083ab612c7564a",
					"CALCULATE32.PNG.1086":                   "863c17297cf0256b560149d5f3f0f9fb6b331908a85a3020a1a265f4ec98aa74",
					"CLEARFILTER16.PNG.1086":                 "d0cff737b049bdd8404914f65b12fe8b8e690458f02b4d558ab5f063324d472e",
					"CLOSEPH.PNG.1086":                       "0f3ef24ac48b3dfce987b374eafadb166aede30321e6729bd7dfe40942999d9e",
					"DATAFEED.RES_1086":                      "2af045557a831dc3195733fa4a44dab81b259889f7ff8d37fb98ee8e3bba226b",
					"DELETESHEET32.PNG.1086":                 "b0274c4c61910990d4b8b817e7711d0b8853609ad3665b7a7387e34ea0220c04",
					"DOWNLOADACOPYPH.PNG.1086":               "7bcf17f19ab582e90662434bc6f87c7db3656f8e8704467d5468724b6ea73c66",
					"DOWNLOADSNAPSHOT32.PNG.1086":            "836278fc5c69a5abd4e46d9128b36ee707d383ef2077a747b62d44ecb4392d40",
					"DOWNLOADTOEXCEL32.PNG.1086":             "5c3a8a51b0b65ca9e08c476d5b01da5e6d6d4a36085fc2f3c3d3391b217a0341",
					"EWACOMMON.IMAGE.CSS.1086":               "676828e53b848e2d58b29f24e0d406a8cbad4ffb165c33ae66f758bf33e15b46",
					"EWACOMMON.PNG.1086":                     "db7cdfe75dd6c59235e73d67eafe5690714747bd820a4b6f35b646f97f19d547",
					"EWAEDIT.PNG.1086":                       "68c4b1f500846e58c4752107729d67b841dd3d70ff82782e066aee874404505f",
					"EWAIS.PNG.1086":                         "40d044f8a66e89a3d2ca01de69c125ea576514d5db26acd4565e740d8441f4e9",
					"EWAMENU.PNG.1086":                       "e62f18b96abb57fbb8d99421bdbdf77bf5c4471adb3521b0594b32979fda7504",
					"EWAOTHER.PNG.1086":                      "3274c227293466dc0db13e59be9c7ca460390a6249123fca09a72217c63a18b8",
					"EWR010.PNG.1086":                        "443894837e60a1c33c20163fc99b1fb51ca0546f4d66bc753c70dc0230fb6f39",
					"EWR010RTL.PNG.1086":                     "ee40e602d8c09878f9ce21166dc06555bccbc9eacf0c6e12e9dffc7f41f04854",
					"EWR034.GIF.1086":                        "b1442e85b03bdcaf66dc58c7abb98745dd2687d86350be9a298a1d9382ac849b",
					"EWR043.PNG.1086":                        "71e893fb51eb4ec0ef67f4fbddc7f593302615d2518a8ab77dc5cfe0610160f2",
					"EWR044.PNG.1086":                        "27e4b6f1cbdc8a013012eabab767aaa1c5650b055c4bf8d3b05d326ba8cfed20",
					"EWR048.GIF.1086":                        "02b7986aca9c34c01da5c88bc8b546393f15b894a5be436c1b761b5c8ce7eaff",
					"EWR049.GIF.1086":                        "78e575bd20b20617f8cbe56b6ea5f2f4c391814ba2b0c5b3e582e87e8a269370",
					"EWR051.PNG.1086":                        "eff5054c4c05d3af67d83322cf2a7e13807958cc975c498ba021567f5a28db13",
					"EWR074.PNG.1086":                        "2d6903823e869f5c7398e6d931fc6d6b7733df8085cfb36267ff4592418c1dbd",
					"EWR075.PNG.1086":                        "7ec839771dd1dbc02a6f840a203c5bda3a17366fcad814251026efb2b0ce4951",
					"EWR076.PNG.1086":                        "7ec839771dd1dbc02a6f840a203c5bda3a17366fcad814251026efb2b0ce4951",
					"EWR077.PNG.1086":                        "7ec839771dd1dbc02a6f840a203c5bda3a17366fcad814251026efb2b0ce4951",
					"EWR080.PNG.1086":                        "ade718134666829ed5ac55c8c5183773228d9f34ffc2131d9593bcb224e1697d",
					"EWR095.PNG.1086":                        "4114d8d01676eedf00edd9195e5c87feb012b3dbd98c709697a63d2229ea2db5",
					"EWR096.PNG.1086":                        "1a1f7613a6ae03d25d250e1737a12e29f7a8725bd36dfeaeabaf2309b8b0cef9",
					"EWR097.PNG.1086":                        "6222331ab667ec184c4cd40961b1715ee41dca7c17b6ba289202c3a6e885edfb",
					"EWR098.PNG.1086":                        "64e3fb9c37a10e9e70e79371e8cff82eff7941d4f5af1702d8a44843920ca029",
					"EWR099.PNG.1086":                        "d749bd4ab4c86966fba946629cc38eebfe5be5d4278fc37c30e363a3d1b64965",
					"EWR100.PNG.1086":                        "753c2cfb3c2fa7d19cea85d60e8fe9542b4877927b594a518080b7480402e198",
					"EWR101.PNG.1086":                        "9b1b6c52f18a0aac4effb73617874bcf8d15a9e5e4deac98f85628b80e47f6fe",
					"EWR102.PNG.1086":                        "38637ab03f02c0df48a0a0b2ef5786882c8e401b1c8c254cc4f2362cfe60fc2e",
					"EWR117.PNG.1086":                        "bbbc950901df24186680829878eb6e3f19b341cdf2d85439912c6594d9afca70",
					"EWR136.PNG.1086":                        "4bd3306c3c09b4fa7dc67ab42b64a558fe3f069466c6b755038798b62718d84d",
					"EWRDEFAULT.CSS.1086":                    "7060bff7d55b40c512531099cbf9dc9a3648b71e96a904a108ab9e47696ecbff",
					"EWRNOV.CSS.1086":                        "1b1e82784749fb4845f8012a0e1877aa9f0df0ba695a9be1160ee8a3dfe5d2a0",
					"EXCEL32.PNG.1086":                       "c6425d3dd4be560c7a21e6d46342c5728efe44470aad342b192bfb34563d9908",
					"EXCELEVERYWHERE.XLSX.1086":              "41bf9f302bb4d9db79f8ee63af27d1c8ba6204412d77ba0573dccc7243b9ca62",
					"FIELDLISTPANE.CSS.1086":                 "c62b145d82d73d34557cf5a01ae1d218da4b02e75b54f04da64641f03fd81892",
					"GIVEFEEDBACK32.PNG.1086":                "eec5def86c62061c79f7fcc3e6eaee2e9003343d20a15241110e71122d29198a",
					"INFO32.PNG.1086":                        "640a468fe0ba457e93b1ee14b08e8276e2a67cff66554189b6ab34a4489dcc15",
					"LAYOUTSRES.XLSRV.LLCC.RESX.1086":        "5987f36732c0466d67f275d023ebd80fc1d842a249aaad7f82aff9117425ec94",
					"MICROSOFT.OFFICE.EXCEL.WEBUI.MOBILE.INTL.RESOURCES.DLL.1086":  "c3880de9aac1e873db3f030bd4bc25e1108e333ed3fc75d86eab15f97294b706",
					"MICROSOFT.OFFICE.EXCEL.WEBUI.MOBILEA.INTL.RESOURCES.DLL.1086": "319ffe52245e1a75abd6dfa58311f3f6ed93f887eafcee5a2212a5a46ad04768",
					"MOBILEWAC.MEWA.PNG.1086":                                      "94de78c3dab8acc28bb46de122dfc0847a6553c7b5844768d0ca0c5b1013d678",
					"MSOXLSRV.RESOURCES.DLL.1086":                                  "13dd22222a7b2701a0cf67e35ccd61931536d887f5f357f945be95ad49838c0f",
					"MWAC.MEWA.STRING.JS.1086":                                     "1496cc91a2f2faf49c0cef6056d1cdf3d068ab6ed58d3007def4cb40e3972de1",
					"MWAC.MOBILEEWA.IMAGE.CSS.1086":                                "ac463fb8b26d76b173a738166d4f31265314b2a1f8f73007b094aba027d2c2bd",
					"OFFICE2010.PNG.1086":                                          "0ed03f4de40ac528d8d286e294333cfb97b3511a1bfcf78ee50f86a44a95a728",
					"OSFRUNTIME_STRINGS.JS.1086":                                   "d68bb57b64bec6f7f269268893eb5f38368252a42e2041cee045fe4e7ad6a1ab",
					"PROPERTIESHH.PNG.1086":                                        "170639b06969be6801597879844990f5ad649a9766db3c9c850ad7fb9bac9498",
					"PROTECTFORMHH.PNG.1086":                                       "fdf87fceae8d670fc8a9e3400776c9b08af61b431c97070893bff244d4448dbd",
					"REFRESHALL32.PNG.1086":                                        "1807cad3186735c4ecb4f11a1ab57c2981af63f85bffc79bea88ab2271c096b9",
					"REFRESHSELECTED32.PNG.1086":                                   "390c3310b2b7a6070ed7dd97c9476d6fa9f63e7e8470aec0e31aa0b29099bd52",
					"RISKS16.PNG.1086":                                             "a3a949319ad05cd38c2a45a91c19893e38e05a69a2b59cef99247a9d6655329c",
					"SAVEAS32.PNG.1086":                                            "3340e2b1292e6b425aaed360f9bc62228121c908568fdd6cf3f746d73ace0004",
					"SHAREDOCHH.PNG.1086":                                          "ee7720b25392770f5a32281e246fced53fd8226eaaa0456b976719a82a2ef2ee",
					"SLICERS.CSS.1086":                                             "fc070905cda3ebd835c65d731a43323464d6463b69dab56c823cb7b2759a3735",
					"STREAMING.RES_1086":                                           "4caf9be5855843cc2c430aece086259768bbae23a107d17c7147c80b273b9f9d",
					"UPDATE32.PNG.1086":                                            "24fe4113cf66955e6bf8d027f5044820e6254d4161bd2033dbbcdf68a4ab31b0",
					"V14_BIEXPLORE.CSS.1086":                                       "bf85da996e873eb37c9a47cc67e9bb6022c147042356cc0c39372522bbda8338",
					"V14_BISERVER.INTL.COMMONINTL.JS.1086":                         "3165e2eeaaca351f97eb4e8f08f3059f0f421d06f5dcf1ae753e26f650025d90",
					"V14_BISERVER.INTL.EWA.READTOOLBAR.JS.1086":                    "432c0678fd533e6b6e409a25a0e1ad64c2975c386f5736a6adf06564ed9dc21e",
					"V14_BISERVER.INTL.EWA.RIBBON.JS.1086":                         "acc9c443c8dfa96824f0cc129fde32e7531740c9edeea5a8cde1670e1a3ac58a",
					"V14_BISERVER.INTL.EWA.STATUSBAR.JS.1086":                      "7c0634c5cc2e2a616730acdd56be35f6f5aa7a316038baa1a3e19e796ca88246",
					"V14_BISERVER.INTL.EWA.STRINGS.MOSS.JS.1086":                   "95cf3071b41088e74361a90e7057295e915cbc6115d39676d4083ab612c7564a",
					"V14_CALCULATE32.PNG.1086":                                     "863c17297cf0256b560149d5f3f0f9fb6b331908a85a3020a1a265f4ec98aa74",
					"V14_CLEARFILTER16.PNG.1086":                                   "d0cff737b049bdd8404914f65b12fe8b8e690458f02b4d558ab5f063324d472e",
					"V14_CLOSEPH.PNG.1086":                                         "0f3ef24ac48b3dfce987b374eafadb166aede30321e6729bd7dfe40942999d9e",
					"V14_DELETESHEET32.PNG.1086":                                   "b0274c4c61910990d4b8b817e7711d0b8853609ad3665b7a7387e34ea0220c04",
					"V14_DOWNLOADACOPYPH.PNG.1086":                                 "7bcf17f19ab582e90662434bc6f87c7db3656f8e8704467d5468724b6ea73c66",
					"V14_DOWNLOADSNAPSHOT32.PNG.1086":                              "836278fc5c69a5abd4e46d9128b36ee707d383ef2077a747b62d44ecb4392d40",
					"V14_DOWNLOADTOEXCEL32.PNG.1086":                               "5c3a8a51b0b65ca9e08c476d5b01da5e6d6d4a36085fc2f3c3d3391b217a0341",
					"V14_EWACOMMON.PNG.1086":                                       "db7cdfe75dd6c59235e73d67eafe5690714747bd820a4b6f35b646f97f19d547",
					"V14_EWAEDIT.PNG.1086":                                         "68c4b1f500846e58c4752107729d67b841dd3d70ff82782e066aee874404505f",
					"V14_EWAIS.PNG.1086":                                           "40d044f8a66e89a3d2ca01de69c125ea576514d5db26acd4565e740d8441f4e9",
					"V14_EWAMENU.PNG.1086":                                         "e62f18b96abb57fbb8d99421bdbdf77bf5c4471adb3521b0594b32979fda7504",
					"V14_EWAOTHER.PNG.1086":                                        "3274c227293466dc0db13e59be9c7ca460390a6249123fca09a72217c63a18b8",
					"V14_EWR010.PNG.1086":                                          "443894837e60a1c33c20163fc99b1fb51ca0546f4d66bc753c70dc0230fb6f39",
					"V14_EWR010RTL.PNG.1086":                                       "ee40e602d8c09878f9ce21166dc06555bccbc9eacf0c6e12e9dffc7f41f04854",
					"V14_EWR034.GIF.1086":                                          "b1442e85b03bdcaf66dc58c7abb98745dd2687d86350be9a298a1d9382ac849b",
					"V14_EWR043.PNG.1086":                                          "71e893fb51eb4ec0ef67f4fbddc7f593302615d2518a8ab77dc5cfe0610160f2",
					"V14_EWR044.PNG.1086":                                          "27e4b6f1cbdc8a013012eabab767aaa1c5650b055c4bf8d3b05d326ba8cfed20",
					"V14_EWR048.GIF.1086":                                          "02b7986aca9c34c01da5c88bc8b546393f15b894a5be436c1b761b5c8ce7eaff",
					"V14_EWR049.GIF.1086":                                          "78e575bd20b20617f8cbe56b6ea5f2f4c391814ba2b0c5b3e582e87e8a269370",
					"V14_EWR051.PNG.1086":                                          "eff5054c4c05d3af67d83322cf2a7e13807958cc975c498ba021567f5a28db13",
					"V14_EWR074.PNG.1086":                                          "2d6903823e869f5c7398e6d931fc6d6b7733df8085cfb36267ff4592418c1dbd",
					"V14_EWR075.PNG.1086":                                          "7ec839771dd1dbc02a6f840a203c5bda3a17366fcad814251026efb2b0ce4951",
					"V14_EWR076.PNG.1086":                                          "7ec839771dd1dbc02a6f840a203c5bda3a17366fcad814251026efb2b0ce4951",
					"V14_EWR077.PNG.1086":                                          "7ec839771dd1dbc02a6f840a203c5bda3a17366fcad814251026efb2b0ce4951",
					"V14_EWR080.PNG.1086":                                          "ade718134666829ed5ac55c8c5183773228d9f34ffc2131d9593bcb224e1697d",
					"V14_EWR095.PNG.1086":                                          "4114d8d01676eedf00edd9195e5c87feb012b3dbd98c709697a63d2229ea2db5",
					"V14_EWR096.PNG.1086":                                          "1a1f7613a6ae03d25d250e1737a12e29f7a8725bd36dfeaeabaf2309b8b0cef9",
					"V14_EWR097.PNG.1086":                                          "6222331ab667ec184c4cd40961b1715ee41dca7c17b6ba289202c3a6e885edfb",
					"V14_EWR098.PNG.1086":                                          "64e3fb9c37a10e9e70e79371e8cff82eff7941d4f5af1702d8a44843920ca029",
					"V14_EWR099.PNG.1086":                                          "d749bd4ab4c86966fba946629cc38eebfe5be5d4278fc37c30e363a3d1b64965",
					"V14_EWR100.PNG.1086":                                          "753c2cfb3c2fa7d19cea85d60e8fe9542b4877927b594a518080b7480402e198",
					"V14_EWR101.PNG.1086":                                          "9b1b6c52f18a0aac4effb73617874bcf8d15a9e5e4deac98f85628b80e47f6fe",
					"V14_EWR102.PNG.1086":                                          "38637ab03f02c0df48a0a0b2ef5786882c8e401b1c8c254cc4f2362cfe60fc2e",
					"V14_EWR117.PNG.1086":                                          "bbbc950901df24186680829878eb6e3f19b341cdf2d85439912c6594d9afca70",
					"V14_EWR136.PNG.1086":                                          "4bd3306c3c09b4fa7dc67ab42b64a558fe3f069466c6b755038798b62718d84d",
					"V14_EWRDEFAULT.CSS.1086":                                      "7060bff7d55b40c512531099cbf9dc9a3648b71e96a904a108ab9e47696ecbff",
					"V14_EWRNOV.CSS.1086":                                          "1b1e82784749fb4845f8012a0e1877aa9f0df0ba695a9be1160ee8a3dfe5d2a0",
					"V14_EXCEL32.PNG.1086":                                         "c6425d3dd4be560c7a21e6d46342c5728efe44470aad342b192bfb34563d9908",
					"V14_FIELDLISTPANE.CSS.1086":                                   "c62b145d82d73d34557cf5a01ae1d218da4b02e75b54f04da64641f03fd81892",
					"V14_GIVEFEEDBACK32.PNG.1086":                                  "eec5def86c62061c79f7fcc3e6eaee2e9003343d20a15241110e71122d29198a",
					"V14_INFO32.PNG.1086":                                          "640a468fe0ba457e93b1ee14b08e8276e2a67cff66554189b6ab34a4489dcc15",
					"V14_OFFICE2010.PNG.1086":                                      "0ed03f4de40ac528d8d286e294333cfb97b3511a1bfcf78ee50f86a44a95a728",
					"V14_PROPERTIESHH.PNG.1086":                                    "170639b06969be6801597879844990f5ad649a9766db3c9c850ad7fb9bac9498",
					"V14_PROTECTFORMHH.PNG.1086":                                   "fdf87fceae8d670fc8a9e3400776c9b08af61b431c97070893bff244d4448dbd",
					"V14_REFRESHALL32.PNG.1086":                                    "1807cad3186735c4ecb4f11a1ab57c2981af63f85bffc79bea88ab2271c096b9",
					"V14_REFRESHSELECTED32.PNG.1086":                               "390c3310b2b7a6070ed7dd97c9476d6fa9f63e7e8470aec0e31aa0b29099bd52",
					"V14_RISKS16.PNG.1086":                                         "a3a949319ad05cd38c2a45a91c19893e38e05a69a2b59cef99247a9d6655329c",
					"V14_SAVEAS32.PNG.1086":                                        "3340e2b1292e6b425aaed360f9bc62228121c908568fdd6cf3f746d73ace0004",
					"V14_SHAREDOCHH.PNG.1086":                                      "ee7720b25392770f5a32281e246fced53fd8226eaaa0456b976719a82a2ef2ee",
					"V14_SLICERS.CSS.1086":                                         "fc070905cda3ebd835c65d731a43323464d6463b69dab56c823cb7b2759a3735",
					"V14_UPDATE32.PNG.1086":                                        "24fe4113cf66955e6bf8d027f5044820e6254d4161bd2033dbbcdf68a4ab31b0",
					"V14_WAC.INTL.FRAME.CSS.1086":                                  "6efcc1098340c08974a0980439c7e24bf8e3bd296b5452d58ef2f37836c74723",
					"V14_XLSRV.PROGRESS.GIF.1086":                                  "a3596c17dad9a003d0bfbe0b7ba6765f51391b5c3943660316f01c8e77b323db",
					"V14_XLSRV.PROGRESS16.GIF.1086":                                "38e88b6af6c6531959a5ad70f5310b60878dc948086a1d4107168b08cc44ecf7",
					"WAC.INTL.FRAME.CSS.1086":                                      "6efcc1098340c08974a0980439c7e24bf8e3bd296b5452d58ef2f37836c74723",
					"XLMSMDSR.RLL_1086":                                            "12f61795e59c1beb71e884a936020dacf3b5645c1c063f71b51e09908c2797af",
					"XLSRV.LLCC.RESX.1086":                                         "5987f36732c0466d67f275d023ebd80fc1d842a249aaad7f82aff9117425ec94",
					"XLSRV.PROGRESS.GIF.1086":                                      "a3596c17dad9a003d0bfbe0b7ba6765f51391b5c3943660316f01c8e77b323db",
					"XLSRV.PROGRESS16.GIF.1086":                                    "38e88b6af6c6531959a5ad70f5310b60878dc948086a1d4107168b08cc44ecf7",
					"XLSRVINTL.DLL.1086":                                           "4a5f5bc34e972c95bf68699f333fa76b4c333ba2bbf9bf1f63c48d0f926917df",
					"XLXMLA.RES_1086":                                              "08c19867398521e8eaad51d2ea3ecc33e2f7bab5606c611972a6b97fa64c02e7",
				},
			},
		},
		{
			update: &update{
				id:         "138ECA2DEB45E284DC0BB94CC8849D1933B072FF",
				remotePath: "138ECA2DEB45E284DC0BB94CC8849D1933B072FF.cab",
				format:     cabArchive,
			},
			testdataPath: "testdata/138ECA2DEB45E284DC0BB94CC8849D1933B072FF.cab",
			wantExtraction: extraction{
				update: "138ECA2DEB45E284DC0BB94CC8849D1933B072FF",
				files: map[string]string{
					"e16k7iws.cat": "05cb353ce92390e77297e596aed8d94f2ba8b39df8f5bd3fed1cd390f1660c22",
					"e16k7iws.inf": "61156dcaaca8a1bd547adfbd40ab4fd6542de31f583eac5cc74b297f8bc34ef4",
					"e16k7iws.rom": "073e26a911a5c611533c8deb5554f8e00a44782f2e688ef4b808b73b3a21ef0f",
				},
			},
		},
	} {
		bytes, err := ioutil.ReadFile(tc.testdataPath)
		if err != nil {
			t.Fatalf("could not open test GCP file: %v", err)
		}

		storageHTTPClient, storageHTTPTransport := mockHTTPClientAndTransport()
		storageHTTPTransport.add(200, string(bytes), nil)
		storageClient, err = storage.NewService(ctx, option.WithHTTPClient(storageHTTPClient))
		if err != nil {
			t.Fatalf("could not create mock GCE client: %v", err)
		}

		extractionDir, err := tc.update.Preprocess()
		if err != nil {
			t.Fatalf("Unexpected error while running Preprocess(): %v", err)
		}

		var extractedFiles []string
		err = filepath.Walk(extractionDir, func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}
			extractedFiles = append(extractedFiles, path)
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error while traversing extraction directory: %v", err)
		}

		files := make(map[string]string)
		for _, file := range extractedFiles {
			hash, err := sha256sum(file)
			if err != nil {
				t.Fatalf("unexpected error while hashing extracted files: %v", err)
			}
			_, filename := filepath.Split(file)
			files[filename] = fmt.Sprintf("%x", hash)
		}
		gotExtraction := extraction{update: tc.update.ID(), files: files}

		if !cmp.Equal(tc.wantExtraction, gotExtraction, cmp.AllowUnexported(extraction{})) {
			t.Errorf("Preprocess() unexpected diff (-want/+got):\n%s", cmp.Diff(tc.wantExtraction, gotExtraction, cmp.AllowUnexported(extraction{})))
		}
	}
}

func sha256sum(path string) ([32]byte, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return [32]byte{}, err
	}

	return sha256.Sum256(data), nil
}
