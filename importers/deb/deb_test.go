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

package deb

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func sha256sum(path string) ([32]byte, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return [32]byte{}, err
	}

	return sha256.Sum256(data), nil
}

func testImages() ([]*Archive, error) {
	debRepo := NewRepo("testdata")
	gotSources, err := debRepo.DiscoverRepo()
	if err != nil {
		return nil, fmt.Errorf("unexpected error while discovering repo: %v", err)
	}

	images := []*Archive{}
	for _, source := range gotSources {
		if image, ok := source.(*Archive); ok {
			images = append(images, image)
		} else {
			return nil, errors.New("error while casting Source interface to Image struct")
		}
	}

	return images, nil
}

func TestDiscover(t *testing.T) {
	gotImages, err := testImages()
	if err != nil {
		t.Fatal(err)
	}

	wantImages := []*Archive{
		{
			filename:   "ubuntu-desktop.deb",
			remotePath: "testdata/20200106.00.00/ubuntu-desktop.deb",
			repoPath:   "testdata",
		},
		{
			filename:   "ubuntu-laptop.deb",
			remotePath: "testdata/20200106.00.00/ubuntu-laptop.deb",
			repoPath:   "testdata",
		},
		{
			filename:   "ubuntu-server.deb",
			remotePath: "testdata/20200106.00.00/ubuntu-server.deb",
			repoPath:   "testdata",
		},
		{
			filename:   "ubuntu-desktop.deb",
			remotePath: "testdata/20200107.00.00/ubuntu-desktop.deb",
			repoPath:   "testdata",
		},
		{
			filename:   "ubuntu-laptop.deb",
			remotePath: "testdata/20200107.00.00/ubuntu-laptop.deb",
			repoPath:   "testdata",
		},
		{
			filename:   "ubuntu-server.deb",
			remotePath: "testdata/20200107.00.00/ubuntu-server.deb",
			repoPath:   "testdata",
		},
		{
			filename:   "ubuntu-desktop.deb",
			remotePath: "testdata/20200107.01.00/ubuntu-desktop.deb",
			repoPath:   "testdata",
		},
		{
			filename:   "ubuntu-laptop.deb",
			remotePath: "testdata/20200107.01.00/ubuntu-laptop.deb",
			repoPath:   "testdata",
		},
		{
			filename:   "ubuntu-server.deb",
			remotePath: "testdata/20200107.01.00/ubuntu-server.deb",
			repoPath:   "testdata",
		},
		{
			filename:   "ubuntu-desktop.deb",
			remotePath: "testdata/20200108.00.00/ubuntu-desktop.deb",
			repoPath:   "testdata",
		},
		{
			filename:   "ubuntu-laptop.deb",
			remotePath: "testdata/20200108.00.00/ubuntu-laptop.deb",
			repoPath:   "testdata",
		},
		{
			filename:   "ubuntu-server.deb",
			remotePath: "testdata/20200108.00.00/ubuntu-server.deb",
			repoPath:   "testdata",
		},
	}

	if !cmp.Equal(wantImages, gotImages, cmp.AllowUnexported(Archive{})) {
		t.Errorf("Discovery() unexpected diff (-want/+got):\n%s", cmp.Diff(wantImages, gotImages, cmp.AllowUnexported(Archive{})))
	}
}

func TestQuickHash(t *testing.T) {
	gotImages, err := testImages()
	if err != nil {
		t.Fatal(err)
	}

	gotHashes := make(map[string]string)
	for _, image := range gotImages {
		hash, _ := image.QuickSHA256Hash()
		gotHashes[image.remotePath] = hash
	}

	wantHashes := map[string]string{
		"testdata/20200106.00.00/ubuntu-laptop.deb":  "a61904b9f97e6eef41586f416657045dcd4141d751600cc89e135103279e77a1",
		"testdata/20200106.00.00/ubuntu-desktop.deb": "8706395a86cdf7e77b433c5a2cba3d096da0a0b86d3fe79e0ac167a31062d658",
		"testdata/20200106.00.00/ubuntu-server.deb":  "9495efbfca5aea3c1fbde35373159623592ab8f60591f7ad73f65f83b49a37e8",
		"testdata/20200107.01.00/ubuntu-laptop.deb":  "e7d97cb016bd39b988b1e44465186c362a2bea63078e50b7968e87d495499974",
		"testdata/20200107.01.00/ubuntu-desktop.deb": "9146742d47b5143a1882749b5458fb6b8fb6cb2ae20429463c4434e33c8744d5",
		"testdata/20200107.01.00/ubuntu-server.deb":  "323a83ac264cf23e96bf56242ccd3c25f78e5eddd74ca8b935ee34c5e954b0d5",
		"testdata/20200108.00.00/ubuntu-laptop.deb":  "6834a6e7c71e7073df814101f6d49f6595f8ebd5db1729adbb2731d5a9f4047a",
		"testdata/20200108.00.00/ubuntu-desktop.deb": "54ffdfaab45c1c3bb70dff41ce0e3412625c7417cff617320552c7fd6bd6fbdf",
		"testdata/20200108.00.00/ubuntu-server.deb":  "911b33a37942821dfdc17983ce9c9f59076ac7f71126d59bc7b367ec883cccf1",
		"testdata/20200107.00.00/ubuntu-laptop.deb":  "f15b61edab68b092001484c8f55872c88b4ee8a0888e5307a6fe1b04e8e7bfd5",
		"testdata/20200107.00.00/ubuntu-desktop.deb": "35a1c9bb833f9af4be0d57dd0799ba85ec4508b69e3e991b35cb8dc362c4ac43",
		"testdata/20200107.00.00/ubuntu-server.deb":  "4f42391fae4724462f9192b44018fc9661c31e81b912fbdef5dd2c2c7dfc55ce",
	}

	if !cmp.Equal(wantHashes, gotHashes) {
		t.Errorf("QuickHash() unexpected diff (-want/+got):\n%s", cmp.Diff(wantHashes, gotHashes))
	}
}

func TestPreprocess(t *testing.T) {
	type extraction struct {
		image string
		files map[string]string
	}

	var gotExtractions []extraction

	images, err := testImages()
	if err != nil {
		t.Fatal(err)
	}

	for _, image := range images {
		extractionDir, err := image.Preprocess()
		if err != nil {
			t.Fatalf("unexpected Preprocess() error: %v", err)
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
		gotExtractions = append(gotExtractions, extraction{image: image.ID(), files: files})
	}

	wantExtractions := []extraction{
		{
			image: "ubuntu-desktop.deb",
			files: map[string]string{
				"file.01": "c2e7f7d23b30766c2d55e847b349d0540f4847b263ee15521dc72023846884ea",
				"file.02": "a74bb803c7ff5bd875867fc3f4ceabb6fbe888eea6361b876111cb8060fe7e8c",
				"file.03": "2789f4b90b038d57e592d01e0cd13a98b398cc7a524c3e8a7faaaaaf59893e7d",
				"file.04": "efa02f852f81f973f2c10784bc5194de1d09f3e905ea296b22090ff3379ed6c1",
				"file.05": "ddf7c381937d07c67e509f18feec42a939bddf2ea7db985a4b045d583c95ec04",
				"file.06": "2fd1880876ca7640d04fae51fa988fe40505ab15f0f1a05ca6b0b5f09985c82a",
				"file.07": "2cbbbd2fa4045f092ed359cd6632e01e1e45006681949aa98cee7aa0edc6f771",
				"file.08": "8ab37107e0ed8d084afaf252fcdb2e66b99812ab12864b1cd12dfd5a44b25e5e",
				"file.09": "952b39dff291f84b330206a1131c06592c8055800071fc84888c9a3052e51543",
				"file.10": "79f5431b2eecae25c0b29ad8e5d8642d0575b015ed8008ff277dd2308cbdd173",
			},
		},
		{
			image: "ubuntu-laptop.deb",
			files: map[string]string{
				"file.01": "8bc259fd7d49e3a94a2001e7ec276c51736a66167fc90e3453771b0e8e9fc17c",
				"file.02": "3f1ee77b201b6c4f1c37872e363387b247415293853a3f7eed25effee396b68f",
				"file.03": "b9b1fcb88ca7c884c4105c3f9e6f5c782521533ab529b84db41d82241a1b148e",
				"file.04": "b6f453c6cb97193dbf52bdd8423d3c5f6308521af9254cd65f5cb9a777c6b203",
				"file.05": "e6af44bf176b209b8ca050e7834aef2b1b6bcc292acde3c456a8a81d2d47c37c",
				"file.06": "a649460a16c3a2d9a097f93e6e2d0c89c5a52ca5e1cc6d6ca03c64417905753d",
				"file.07": "fa6d182f5bd8613830c118e0d7296baa59ae33b0d32a4557106cd15098b8bcf9",
				"file.08": "f9788be264fc476a842f3e23950a1c0070b47948f95eeccc8c243c45afd62524",
				"file.09": "d370aa6801e91d7fa48db7f4388c4e3858f58de124d9c4fd53cb9a25bbc2fa34",
				"file.10": "b665f633c8f1e184972cb9ebc99490cf999fcae5d520fc8e144ee1124d05628b",
			},
		},
		{
			image: "ubuntu-server.deb",
			files: map[string]string{
				"file.01": "c4013088429ec4cbb0b8c6e62c8260081200f14f427c3f6def1892ea17a1e1d2",
				"file.02": "db378bba4f2aeb9dd8bf155053d188d471dea2aa1eeee867eba35859b3231248",
				"file.03": "aa7178c790d844d39d5950ffc8e4b90e6b6402fd8ab61d3ec2140b9c019f71f3",
				"file.04": "bf28724a6549faeeaf7713cf84028c4a67710a083d38e9085fbb79ac99c0665d",
				"file.05": "8ce9373260d0332f867986ce93ba7ee27ca1177f40262da2532c27be2e2c2e3b",
				"file.06": "0673ad1ccd50967bd6832050a08a0df5d3a4c0dba71faaa3f887c2fa69ce8371",
				"file.07": "f354a000db035fbd924d6be528ce4863fa9596c012f18545fe08f64aa00e2791",
				"file.08": "129c8674ec47634e806e7ae3fda02f32ab227dabc56bcb73e7a8441dddb1077c",
				"file.09": "67eae811b446beb5ed2d16277731203f472de1a957d1580f6a4275d4475050b1",
				"file.10": "2d6538330f5849f3a614c1db845c72ca8312ea20122f94ea7bd2390bbebdd600",
			},
		},
		{
			image: "ubuntu-desktop.deb",
			files: map[string]string{
				"file.01": "13911129c994f843fed92c6deab2d3ae6644d6bee801726b316555d1810c70d5",
				"file.02": "516532ef7e7dbe21befe447e77a0be12debfe8d582990c75fd7d38757b6f891c",
				"file.03": "faba24a2bf832b241b3278e49619dba4c3e5c67c12caf2d6df24f0d8a734b150",
				"file.04": "4aee66f73d018cd369ef77a8db2e77250587613259e959a2d595a59a04dd8a30",
				"file.05": "9ba07fbc067bb8b9d2bed6f808a45e9e3dd321e3dfd1e4d9dcce10753f5b9b51",
				"file.06": "f8035b6497eb22ecabd90f88aed82328cdd7f2daab1b53959f665cdd74a0431e",
				"file.07": "beff39f558826a8349fe6243785669ebfa5f57d47261f3d683ee17fb9be6566a",
				"file.08": "4570a774b4b285c514348ead11078325ff6773c9e5cb8207642404c99df98740",
				"file.09": "d5ae5038f2909618d2bd64c409d3b0ac8f0de3ab8a2d45b974da02633b9f0e40",
				"file.10": "89811409c6ad0b14a8a773c92a5a28971a70a6685031a57c8bd6cbf6a80f7be5",
			},
		},
		{
			image: "ubuntu-laptop.deb",
			files: map[string]string{
				"file.01": "280fc584e14a2b8c74aa2c21d33300d4ae97703087a12cea845229aaf7b9abc1",
				"file.02": "f3afff3f52163c66e72cfade56b210350bf4095bb377d54cb6d09fbaa8f48128",
				"file.03": "6e2c2e9df023d5ea5919e630bfbf070a07226d1b8cd8495f312e3716050c3860",
				"file.04": "9cea0c8afccf5703c79551bd3fa03eba3f2d86fbd26b07f8441bcb096cbbb6f4",
				"file.05": "592c25efd491ae7d78d7146f4b8043d930feb2d8f80e424b701b2cb31aaeffc7",
				"file.06": "d3df0f5cb1cb8889ff9073552980d883e40cbffed16ec0a3abff9449b2138030",
				"file.07": "ff4ae7dc0c5b1461560f35b9ce4714a8f43e3d711da5d7d10a795fac8265b244",
				"file.08": "c8617057c36c6d796684b8b371d404392b4acbbe27504292d1572cd316b426ac",
				"file.09": "e3528da985f1233116d0140c93f110e7a1416e6924716b38a1b02df96f39ab20",
				"file.10": "ab5da92f8e8a364dd43c7110438d65831a5a4efb8be1c86a1ce2e15844cf95ee",
			},
		},
		{
			image: "ubuntu-server.deb",
			files: map[string]string{
				"file.01": "09b1fec24c1e8c22a186b639383a86d577da8007642af3db509becac512eeb4c",
				"file.02": "f61a659e7463abb181db43b7185caf1c44c4229cf4c970b2ef843500e2924592",
				"file.03": "8fce39696330405e3f5befdb18a92ba8aacc12556f7ddf4053367ba617679dd0",
				"file.04": "78747c0da578c721411d4e725d0b906ac57b27c473657f5aea64a9498f438381",
				"file.05": "80cc1c37e306d63567bc92c4b026b858744f5baa90806d6ef39215231a900f0b",
				"file.06": "5749e9588919dfe8912694c7e67ead1ace8edbc854a8d4257a882ffba2efd904",
				"file.07": "4eb1ec9cc85ff46cc6390943882d6794828431f6ddf0b28555b456cb83328919",
				"file.08": "ad7a7ca2679ff5257e834ae4acda9a1a1d6b048c441c088137a5059352fce3f5",
				"file.09": "610a552ab8b44dbc6aebfc548ba289a4b5b6c1e9e9d530aae728c9a3867db469",
				"file.10": "c1650357d2a44187e9cd489dccf902a53fdded14034ff7af788587a43336e2c1",
			},
		},
		{
			image: "ubuntu-desktop.deb",
			files: map[string]string{
				"file.01": "3e720d724145502b0761501f0d918eb45a15434774eee02f8f6a6ddc87eaa2cf",
				"file.02": "2cdea17b0023f6f6292aa1c5141b6accd547eb32a4ea073e2517805ca0900747",
				"file.03": "7fa2c5c87edecfc5f07739ca0dad5d140f922161fb2e5a8caf136e7db8bce91c",
				"file.04": "8bdb6d28e04d7711a6c6d7693ddb2376a112dac4846a8367debb2848847e03c6",
				"file.05": "1bd5c4eaf72be2dc3309469b55409c76aa092ee4e683a8c9eda06a04e15894d4",
				"file.06": "79f7a9846974597b598d864bea012c7832e118912693d59523363df3ee52d5e1",
				"file.07": "972d269f3f9a26581afa646de77f7698da3e990d30e7b5b1fbad66eecf07b803",
				"file.08": "ec8284d58abefc0a82e490641031d4a88cee7d8bdc09cef7b5fd8f18dede6e8a",
				"file.09": "8a95a96a002834ceedb1dcf7c10c8b2010f27a946ebb14f75b63f24eee8789e2",
				"file.10": "26a691931baa193647481207686b92b58e7064520d604014ba72afdc12f218cd",
			},
		},
		{
			image: "ubuntu-laptop.deb",
			files: map[string]string{
				"file.01": "ff9a95c3c460d65453a3abc30126758c07b4d35a960c11e38fb801c9fb76b383",
				"file.02": "753253fe1cf63099e5c05f32b9ff2d7def422814e12298eac4beec6d85dccb60",
				"file.03": "3dcb6600b7a12863a2c5bd183731724819a70cc7513e5a78db4278841a95b2eb",
				"file.04": "4f79ac7abf5cca0b842f73c96a60e129613f6815a3ffa3b1af4cfa5db8c7ab80",
				"file.05": "ac809183432d85d686dbb1ffa4d6261641a31b2a77b494941f14fa2e4e0b9315",
				"file.06": "e92f7e78485166d9882980fce6a78ef73bd23f30902526dedb30ad08f4164809",
				"file.07": "f0169ce59a87329aaec3d7dda41cb3214091086cf2c5d28a7d5deab44e0db8ef",
				"file.08": "da3a5df23487dcef33b16f7f01e20325b646819489fae3cdc2e8f34da4d3f2cd",
				"file.09": "dcc1e672611fc4d5ddbe5f9df041cefc90dc5706535ebf39c5a0a146622db33e",
				"file.10": "704db5c084a9471627ae9a02cf430aad0b05c1cef8c28758536ff4469d8bb009",
			},
		},
		{
			image: "ubuntu-server.deb",
			files: map[string]string{
				"file.01": "df5d51ea487e697da38a1ae8371507798459df3452d34b2b5b96a70ca22d8d05",
				"file.02": "a868d4cc1a81176dfead1e3fb778c16be541c32760dd8bc41e0ede6d0b0137af",
				"file.03": "e33b1aec41f596bd0d02e5ef60350166fef83c3e65ea968286e4bd66e64ad03c",
				"file.04": "14f0c6763b8c50703547bf1ddd6dc367238e0592c4142115378ea72daa86655c",
				"file.05": "323259850a60979ae147edaf877447c456d6f0cfa96e91e919d175066340cc99",
				"file.06": "24ac3b8a9c1b9e3e48862f5253aa895ae1a2c70b24108c6e3f280f92e8f28b85",
				"file.07": "fc08b4aeea6748f0c5d1a2bdc751bd93cc3d72a59214c91a201edcaf6c5deea4",
				"file.08": "5de2e0bbfbb17152a59c480974593d306947146df873be5a52162db9261496bb",
				"file.09": "4047ba956f12271ab680c7039ec6271acc168dfe88cde1938ee7066c61de62db",
				"file.10": "dbf21781a7f20f1651cb7af67aeb82d2382e713c2a6bc41e9906679ef2463ba4",
			},
		},
		{
			image: "ubuntu-desktop.deb",
			files: map[string]string{
				"file.01": "9f114adf6f76f6c2f403699c31b8eadb168c5fd0ee71e423ae29b99251c2595e",
				"file.02": "4878dd6c7af7fecdf89832384d84ed93b781d3e69e6a0097efac5320da2ac637",
				"file.03": "9862cfa4e6b71b6d11e13c5392cd3607c96a279e1e22a4535cf4d019055494d1",
				"file.04": "0a632850049f80763ada81ec0cacf015dbd67fb1b956ec2acb8aa862e511b3bc",
				"file.05": "471ed5b2f059540ae4d8586a47329272c774561fc1a9c15fb22f2d7939b6820b",
				"file.06": "b8862d9e62c15c73527ca72b4e5e85809d4254326800eb2c65b35339029e02d1",
				"file.07": "733dac1b5272600122760db1f917bb4729d69e5a69f039797f75e685a1152bf0",
				"file.08": "c8c6fb7771c086f786ea8d60bf703bbeea3ed7984ce962493994493839f70713",
				"file.09": "d5de3fe6a4559c59ad103ab40e01c4fc0df7eb8ba901d50e5ceae3909b2e0d61",
				"file.10": "05d1223b8b287684ce015d918b90555f9cb4c753558d511d179ada227820f6ba",
			},
		},
		{
			image: "ubuntu-laptop.deb",
			files: map[string]string{
				"file.01": "db391f171b17f750e440c6311b8ef618705d14f0de99eedff954b318967d9e1a",
				"file.02": "f99254f3f48cf49195f0de6ce59257634e2c9a1c251c7013d7ba246ed6ec841d",
				"file.03": "d15a7d5f9731738452cd67b6bfa035e4b12b3d934d3b47d37c1c8273eeaa23fd",
				"file.04": "15a11e8e69ba8203df18c1f2baaee4ffb7de4be7d4abb4bd86d7e232231aeaae",
				"file.05": "c84f7f5db9e404bafe87bc4123474801486ad8f65ca63de4f790bb885c99e25d",
				"file.06": "ff0ca17a9756bb3d91b748887ef2d5fe7e97137e3f5b1f6b8ec6cc7fe369f8fd",
				"file.07": "229faa1edf71fa3b221d17f5d90d852029c2559f91f53eb464036d9b0a197eee",
				"file.08": "c8fc201d41aeb679d8da4708491e7ed4ba3a4e3be85c50210b08a2372284624a",
				"file.09": "ba1b175b82ea3493bb2c0c0bca529aaeb42ce5cf6888cb09f7df8e87a639beae",
				"file.10": "7ca80a863d5c935068511d939ad8857f6c0e1fe1bf62ff4160b1a78bba84e3ba",
			},
		},
		{
			image: "ubuntu-server.deb",
			files: map[string]string{
				"file.01": "9076527a1200b53570b8719f0049b8544c7366003c355cd740858fd3569729f6",
				"file.02": "78d78a3d686c9580a0fb123894f16156a822261a30499f3e8d34cad77496e0fc",
				"file.03": "ebb15f223ca81484e714a08ff93689189214fbb1003f12e007fd8deb98fd6b35",
				"file.04": "cd1be623861c325203d1582826a5bffdbc732fa5f86984d8422c2623d45c3164",
				"file.05": "207b2c6bc867f366e2f7d7607662bd9329e84c68a97affba23963e9a4245ef01",
				"file.06": "f481bbd1719e1adb5446b1cb0df10b7564edfdc170cc5e2afa4b72b74a56788c",
				"file.07": "807034689f2c2b179b60692162bf99eb390d39a2b39e6897949b030805e953d2",
				"file.08": "3665d702279c54659944ba9def060306f261d2706793ddbc9f6214a377c36a4c",
				"file.09": "ebd093091d65f012ebb1c0d6128334984924132e865a98b6fa6349f143102811",
				"file.10": "40998bbd7e456cb57beacc7006eb5c949160527fb6589c8fce3b1f49a1a9f3bd",
			},
		},
	}

	if !cmp.Equal(wantExtractions, gotExtractions, cmp.AllowUnexported(extraction{})) {
		t.Errorf("Preprocess() unexpected diff (-want/+got):\n%s", cmp.Diff(wantExtractions, gotExtractions, cmp.AllowUnexported(extraction{})))
	}
}

func TestImageFunctions(t *testing.T) {
	id := "ubuntu-desktop.deb"
	repoPath := "/tmp/deb-repo"
	localTarGzPath := "/tmp/deb-repo/20200108.00.00-ubuntu-desktop.deb"
	remotePath := "/share/deb-repo/20200108.00.00-ubuntu-desktop.deb"

	img := Archive{filename: id, localPath: localTarGzPath, remotePath: remotePath, repoPath: repoPath}

	if img.ID() != id {
		t.Errorf("ID() = %s; want = %s", img.ID(), id)
	}

	if img.RepoName() != RepoName {
		t.Errorf("RepoName() = %s; want = %s", img.RepoName(), RepoName)
	}

	if img.RepoPath() != repoPath {
		t.Errorf("RepoPath() = %s; want = %s", img.RepoPath(), repoPath)
	}

	if img.LocalPath() != localTarGzPath {
		t.Errorf("LocalPath() = %s; want = %s", img.LocalPath(), localTarGzPath)
	}

	if img.RemotePath() != remotePath {
		t.Errorf("RemotePath() = %s; want = %s", img.RemotePath(), remotePath)
	}
}

func TestRepoFunctions(t *testing.T) {
	repoPath := "/tmp/deb-repo"
	repo := NewRepo(repoPath)

	if repo.RepoName() != RepoName {
		t.Errorf("RepoName() = %s; want = %s", repo.RepoName(), RepoName)
	}

	if repo.RepoPath() != repoPath {
		t.Errorf("RepoPath() = %s; want = %s", repo.RepoPath(), repoPath)
	}
}
