// predict project main.go
package main

import (
	"C"
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/gocolly/colly"
	"github.com/mfonda/simhash"
	"github.com/nathanleary/js/go-reflector"
	"golang.org/x/net/html"
	// "github.com/nathanleary/convnet"
	"github.com/nathanleary/js/php2go"
	"github.com/spf13/cast"
	"github.com/valyala/fasthttp"
)
import (
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"log"

	"github.com/nfnt/resize"
	"golang.org/x/image/bmp"
)

var model *autocomplete = new(autocomplete)
var iterations int = 5
var mut *sync.RWMutex = new(sync.RWMutex)
var trainingCycles int = 0.0
var trainingSessions int = 0.0
var wait bool = false

type autocomplete struct {
	dirfile string
	max     float64
	total   float64
	count   float64
	data    map[string]map[string]map[string]int
}

func stringify(s interface{}) string {
	fltB, _ := json.Marshal(s)
	return (string(fltB))
}

func jsonparse(s string) interface{} {
	var fltB interface{}
	json.Unmarshal([]byte(s), &fltB)
	return fltB
}

func (model *autocomplete) Hash(data string) string {
	return strconv.FormatUint(simhash.Simhash(simhash.NewWordFeatureSet([]byte(data))), 10)
}

func (model *autocomplete) HashCompare(data1, data2 string) uint8 {
	d1, _ := strconv.ParseUint(data1, 10, 64)
	d2, _ := strconv.ParseUint(data2, 10, 64)
	return simhash.Compare(d1, d2)
}

func (model *autocomplete) Md5(data string) string {
	h := md5.New()
	io.WriteString(h, data)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (model *autocomplete) New(dir string) {
	model.dirfile = dir
	model.total = 0.0
	model.max = 0.0
	model.count = 0.0
	model.data = make(map[string]map[string]map[string]int)
}

var forgetQueue []string = []string{}

func (model *autocomplete) Forget(ref string) {

	if len(forgetQueue) > 150 { // not sure how big the forget que should be

		if model.data == nil {
			model.New(model.dirfile)
		}
		ref = forgetQueue[0]
		forgetQueue = forgetQueue[1:]

		model.data[ref] = nil
	}

}

func (model *autocomplete) SaveMicro(path string, ref string) {
	d := model.data
	// fmt.Println(ref, d[ref])
	var network bytes.Buffer        // Stand-in for a network connection
	enc := gob.NewEncoder(&network) // Will write to network.
	mut.Lock()
	mdf := model.data[ref]

	err := enc.Encode(mdf)
	mut.Unlock()
	if err == nil {
		os.MkdirAll(filepath.Dir(path), 0755)

		ioutil.WriteFile(path, network.Bytes(), 0644)
	} else {
		model.data = d
	}
}

func (model *autocomplete) LoadMicro(path string, ref string) {

	hasFoundRef := false

	for x := 0; x < len(forgetQueue); x++ {
		if ref == forgetQueue[x] {
			hasFoundRef = true
			forgetQueue = append(forgetQueue[:x], forgetQueue[x+1:]...)

			x = len(forgetQueue) + 1
		}
	}
	forgetQueue = append(forgetQueue, ref)

	if !hasFoundRef {
		//	fmt.Println(path)
		model.New(model.dirfile)
		dat, err := ioutil.ReadFile(path)
		mdf := make(map[string]map[string]int)
		if err == nil {

			var network bytes.Buffer // Stand-in for a network connection
			//enc := gob.NewEncoder(&network) // Will write to network.
			dec := gob.NewDecoder(&network) // Will read from network.

			//dec := gob.NewDecoder(&network)
			network.Write(dat)
			mut.Lock()

			err = dec.Decode(&mdf)
			//model.data[ref] = mdf

			mut.Unlock()
			if err != nil {
				// model.Update(ref, nil)
				// model.SaveMicro(path, ref)
			}

		} else if err != nil {
			// model.Update(ref, nil)
			// model.SaveMicro(path, ref)

		}

		model.Update(ref, mdf)
	}
}

func (model *autocomplete) Update(ref string, data map[string]map[string]int) {

	// fmt.Println(data)
	if model.data == nil {
		model.New(model.dirfile)
	}

	if data == nil {
		//		model.Forget(ref)
	} else {

		// model.data[ref] = data

		// for i := 0; i < len(models); i++ {
		// for k, _ := range models[i].data {
		// 	if _, ok := model.data[k]; ok {
		k := ref
		for k2, _ := range data {

			if d, ok := model.data[k]; !ok || d == nil {
				if data != nil {
					model.data[k] = data
				} else {
					model.data[k] = make(map[string]map[string]int)
				}
			} else {

				if d2, ok2 := model.data[k][k2]; ok2 && d2 != nil {
					for k3, _ := range data[k2] {
						if _, ok3 := model.data[k][k2][k3]; ok3 {

							model.data[k][k2][k3] = model.data[k][k2][k3] + 1
						} else {
							model.data[k][k2][k3] = data[k2][k3]
						}

					}

				} else {
					if data[k2] != nil {
						if d4, ok4 := model.data[k][k2]; ok4 && d4 != nil {
							model.data[k][k2] = data[k2]
						} else {

							model.data[k][k2] = make(map[string]int)
						}
					}

				}
			}
		}
		// 	} else {

		// 		model.data[k] = models[i].data[k]
		// 	}
		// }
		// }

	}

}

func (model *autocomplete) Punish(input string, autocompleteString string) {
	ref := ""

	for t := 0; t <= 15; t++ {
		if len(input) >= t {
			ref = input[:t]
		}
	}
	//	if len(input) > 3 {
	//		ref = input[:3]
	//	} else if len(input) > 2 {
	//		ref = input[:2]
	//	}

	if ref != "" {
		if _, ok3 := model.data[ref]; ok3 {
			if _, ok := model.data[ref][input]; ok {
				if _, ok2 := model.data[ref][input][autocompleteString]; ok2 {

					model.data[ref][input][autocompleteString] = model.data[ref][input][autocompleteString] - 1

					if model.data[ref][input][autocompleteString] < 0 {

						model.data[ref][input][autocompleteString] = 0

					}
				}
			}
		}
	}
}

var references []string = []string{}
var referencesMap map[string]bool = map[string]bool{}

func (model *autocomplete) DumpMicro(folderPath string) {
	for _, ref := range references {
		if _, ok := model.data[ref]; ok {

			hash := model.Hash(ref)
			fold := fmt.Sprint(model.HashCompare("", hash))
			// fmt.Println(md5, model.data[ref])
			model.SaveMicro(folderPath+"/"+fold+"/"+hash, ref)
			//	model.Forget(ref)
		}
	}

}

func (model *autocomplete) PunishMicro(input string, folderPath string, specificAnswer string, vectorString string) {
	var sync sync.WaitGroup = sync.WaitGroup{}
	models := []autocomplete{}
	//input = strings.TrimSpace(input)
	//	input = strings.Replace(input, vectorString+vectorString+vectorString, vectorString, -1)
	//	input = strings.Replace(input, vectorString+vectorString, vectorString, -1)
	//	input = strings.Replace(input, vectorString+vectorString, vectorString, -1)

	words := strings.Split(input, vectorString)

	//fmt.Println(words)
	for j := len(words) - 1; j >= 0; j-- {

		for z := j + 1; z <= len(words); z++ {

			input = strings.Join(words[j:z], vectorString)
			////input = strings.TrimSpace(input)

			models = append(models, autocomplete{})

			sync.Add(1)
			go func(i int, input string) {

				defer sync.Done()
				tempmodel := autocomplete{}
				mut.RLock()
				d := model.dirfile
				mut.RUnlock()
				tempmodel.New(d)

				tempRef := ""
				for y := 0; y < len(input); y++ {

					for x := y + 1; x <= len(input); x++ {

						ref := ""
						for t := 0; t <= 15; t++ {
							if len(input[y:x]) >= t {
								ref = input[y:x][:t]
							}
						}

						if ref != "" {

							if tempRef != ref || tempRef == "" {
								//								if _, ok := tempmodel.data[ref]; !ok {

								//									// md5 := tempmodel.Md5(ref)
								//									// tempmodel.LoadMicro(folderPath+"/"+md5[:3]+"/"+md5[3:7]+"/"+md5[7:], ref)
								//									//									md5 := tempmodel.Md5(ref)
								//									//									tempmodel.LoadMicro(folderPath+"/"+md5[:3]+"/"+md5[3:7]+"/"+md5[7:], ref)

								//									hash := tempmodel.Hash(ref)
								//									// fmt.Println(md5, tempmodel.data[ref])
								//									fold := fmt.Sprint(tempmodel.HashCompare("", hash))
								//									tempmodel.LoadMicro(folderPath+"/"+fold+"/"+hash, ref)

								//									tempRef = ref
								//									mut.RLock()
								//									if _, ok2 := referencesMap[ref]; !ok2 {
								//										mut.RUnlock()

								//										mut.Lock()
								//										referencesMap[ref] = true
								//										references = append(references, ref)
								//										mut.Unlock()
								//										mut.RLock()
								//									}

								//									mut.RUnlock()
								//								} else {

								//								}
							}

							if _, ok3 := tempmodel.data[ref]; ok3 {

							} else {

								tempmodel.data[ref] = make(map[string]map[string]int)
							}

							specificAnswerTemp := specificAnswer

							if specificAnswer == "" {
								specificAnswerTemp = input[x:]
							}
							if specificAnswerTemp != "" {

								tempmodel.Punish(input[y:x], specificAnswerTemp)

								//								scoreCount := 1 //len(strings.Split(input[x:], " "))
								//								if _, ok4 := tempmodel.data[ref][input[y:x]]; ok4 {

								//									if _, ok2 := tempmodel.data[ref][input[y:x]][specificAnswerTemp]; ok2 {

								//										tempmodel.data[ref][input[y:x]][specificAnswerTemp] += scoreCount

								//									} else {

								//										tempmodel.data[ref][input[y:x]][specificAnswerTemp] = scoreCount

								//									}

								//								} else {

								//									tempmodel.data[ref][input[y:x]] = make(map[string]int)
								//									tempmodel.data[ref][input[y:x]][specificAnswerTemp] = scoreCount

								//								}
								//								fmt.Println(input[y:x], specificAnswerTemp)
							}

						}

					}

				}

				mut.Lock()
				for k, v := range tempmodel.data {

					models[i].Update(k, v)
				}
				mut.Unlock()

			}(len(models)-1, input)
		}

		trainingCycles = trainingCycles + 1
		// if model.max > 1 && trainingCycles%int(model.max*model.max*model.max) == 0 {
		// 	model.Cull()
		// }

		//		fmt.Println(model.max, "max")
		// fmt.Println((model.total/model.count)-((model.max-(model.total/model.count))*2), "min")
		//		fmt.Println(model.total/model.count, "avg")
		//		fmt.Println(float64(j) / float64(len(words)-6))
	}
	// fmt.Println(model.max, "max")
	// fmt.Println((model.total/model.count)-((model.max-(model.total/model.count))*2), "min")
	// fmt.Println(model.total/model.count, "avg")

	sync.Wait()

	for i := 0; i < len(models); i++ {

		for k, v := range models[i].data {
			model.New(model.dirfile)
			hash := model.Hash(k)
			fold := fmt.Sprint(model.HashCompare("", hash))
			model.LoadMicro(model.dirfile+"/"+fold+"/"+hash, k)
			model.Update(k, v)
			model.SaveMicro(model.dirfile+"/"+fold+"/"+hash, k)

			// if _, ok := model.data[k]; ok {
			// 	for k2, _ := range models[i].data[k] {
			// 		if _, ok2 := model.data[k][k2]; ok2 {
			// 			for k3, _ := range models[i].data[k][k2] {
			// 				if _, ok3 := model.data[k][k2][k3]; ok3 {

			// 					model.data[k][k2][k3] = model.data[k][k2][k3] + 1
			// 				} else {
			// 					model.data[k][k2][k3] = models[i].data[k][k2][k3]
			// 				}

			// 			}

			// 		} else {
			// 			model.data[k][k2] = make(map[string]int)
			// 			model.data[k][k2] = models[i].data[k][k2]
			// 		}
			// 	}
			// } else {

			// 	model.data[k] = models[i].data[k]
			// }
		}
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	if bToMb(m.Alloc) > 512 { // if memory exceeds 512mb
		model.DumpMicro(model.dirfile + "/")
		references = []string{}
		referencesMap = map[string]bool{}
		model.New(model.dirfile)
		runtime.GC()
	}

}

func (model *autocomplete) Learn(inputsInterface interface{}, specificAnswersInterface interface{}) {

	inputs := []string{}
	specificAnswers := []string{}

	if i, ok := inputsInterface.(string); ok {
		inputs = append(inputs, i)
	}

	if i, ok := inputsInterface.(interface{}); ok {
		inputs = append(inputs, cast.ToString(i))
	}

	if i2, ok := inputsInterface.([]string); ok {

		inputs = i2
	}

	if i2, ok := inputsInterface.([]interface{}); ok {
		for x := 0; x < len(i2); x++ {
			inputs = append(inputs, cast.ToString(i2[x]))
		}
	}

	if i3, ok := inputsInterface.(*goja.Object); ok {
		for _, k := range i3.Keys() {
			inputs = append(inputs, i3.Get(k).String())
		}
	}

	if i, ok := specificAnswersInterface.(string); ok {
		specificAnswers = append(specificAnswers, i)
	}

	if i, ok := specificAnswersInterface.(interface{}); ok {
		specificAnswers = append(specificAnswers, cast.ToString(i))
	}

	if i2, ok := specificAnswersInterface.([]string); ok {
		specificAnswers = i2
	}

	if i2, ok := specificAnswersInterface.([]interface{}); ok {
		for x := 0; x < len(i2); x++ {
			specificAnswers = append(specificAnswers, cast.ToString(i2[x]))
		}
	}

	if i3, ok := specificAnswersInterface.(*goja.Object); ok {
		for _, k := range i3.Keys() {
			specificAnswers = append(specificAnswers, i3.Get(k).String())
		}
	}

	var sync *sync.WaitGroup = new(sync.WaitGroup)

	models := []autocomplete{}
	//input = strings.TrimSpace(input)
	//	input = strings.Replace(input, vectorString+vectorString+vectorString, vectorString, -1)
	//	input = strings.Replace(input, vectorString+vectorString, vectorString, -1)
	//	input = strings.Replace(input, vectorString+vectorString, vectorString, -1)

	maxIterations := 0
	if len(inputs) >= len(specificAnswers) {
		maxIterations = len(specificAnswers)
	} else {
		maxIterations = len(inputs)
	}

	sync.Add(maxIterations)

	for t := 0; t < maxIterations; t++ {

		input := inputs[t]
		specificAnswer := specificAnswers[t]

		//words := []string{input}

		//for j := len(words) - 1; j >= 0; j-- {

		//for z := j + 1; z <= len(words); z++ {

		//if j < z {
		//fmt.Println(words[j:z])
		//input = strings.Join(words[j:z], vectorString)

		////input = strings.TrimSpace(input)
		spa := specificAnswer
		//	if specificAnswer == "" {
		//		spa = strings.Join(words[z:], vectorString)
		//	}

		models = append(models, autocomplete{})

		go func(i int, input, specificAnswer string) {

			defer sync.Done()

			tempmodel := autocomplete{}
			tempmodel.New(model.dirfile)
			tempRef := ""
			//for y := 0; y < len(input); y++ {

			//for x := y + 1; x <= len(input); x++ {
			x := len(input)
			y := 0

			ref := ""
			for t := 0; t <= 15; t++ {
				if len(input[y:x]) >= t {
					ref = input[y:x][:t]
				}
			}

			if ref != "" {

				if tempRef != ref || tempRef == "" {
					//								if _, ok := tempmodel.data[ref]; !ok {

					//									// md5 := tempmodel.Md5(ref)
					//									// tempmodel.LoadMicro(folderPath+"/"+md5[:3]+"/"+md5[3:7]+"/"+md5[7:], ref)
					//									//									md5 := tempmodel.Md5(ref)
					//									//									tempmodel.LoadMicro(folderPath+"/"+md5[:3]+"/"+md5[3:7]+"/"+md5[7:], ref)

					//									hash := tempmodel.Hash(ref)
					//									// fmt.Println(md5, tempmodel.data[ref])
					//									fold := fmt.Sprint(tempmodel.HashCompare("", hash))
					//									tempmodel.LoadMicro(folderPath+"/"+fold+"/"+hash, ref)

					//									tempRef = ref
					//									mut.RLock()
					//									if _, ok2 := referencesMap[ref]; !ok2 {
					//										mut.RUnlock()

					//										mut.Lock()
					//										referencesMap[ref] = true
					//										references = append(references, ref)
					//										mut.Unlock()
					//										mut.RLock()
					//									}

					//									mut.RUnlock()
					//								} else {

					//								}
				}

				if _, ok3 := tempmodel.data[ref]; ok3 {

				} else {

					tempmodel.data[ref] = make(map[string]map[string]int)

				}

				specificAnswerTemp := specificAnswer

				if specificAnswerTemp != "" {

					tempmodel.JustLearn(input[y:x], specificAnswerTemp)

				}

				//	}

				//}
				mut.Lock()
				for k, v := range tempmodel.data {

					models[i].Update(k, v)
					tempmodel.Forget(k)
				}
				mut.Unlock()

			}

		}(len(models)-1, input, spa)

		//}
		//}

		trainingCycles = trainingCycles + 1
		// if model.max > 1 && trainingCycles%int(model.max*model.max*model.max) == 0 {
		// 	model.Cull()
		// }

		//		fmt.Println(model.max, "max")
		// fmt.Println((model.total/model.count)-((model.max-(model.total/model.count))*2), "min")
		//		fmt.Println(model.total/model.count, "avg")
		//		fmt.Println(float64(j) / float64(len(words)-6))
		//}
		// fmt.Println(model.max, "max")
		// fmt.Println((model.total/model.count)-((model.max-(model.total/model.count))*2), "min")
		// fmt.Println(model.total/model.count, "avg")
	}
	sync.Wait()

	for i := 0; i < len(models); i++ {

		for k, v := range models[i].data {
			model.New(model.dirfile)
			hash := model.Hash(k)
			fold := fmt.Sprint(model.HashCompare("", hash))
			model.LoadMicro(model.dirfile+"/"+fold+"/"+hash, k)
			model.Update(k, v)
			model.SaveMicro(model.dirfile+"/"+fold+"/"+hash, k)

			// if _, ok := model.data[k]; ok {
			// 	for k2, _ := range models[i].data[k] {
			// 		if _, ok2 := model.data[k][k2]; ok2 {
			// 			for k3, _ := range models[i].data[k][k2] {
			// 				if _, ok3 := model.data[k][k2][k3]; ok3 {

			// 					model.data[k][k2][k3] = model.data[k][k2][k3] + 1
			// 				} else {
			// 					model.data[k][k2][k3] = models[i].data[k][k2][k3]
			// 				}

			// 			}

			// 		} else {
			// 			model.data[k][k2] = make(map[string]int)
			// 			model.data[k][k2] = models[i].data[k][k2]
			// 		}
			// 	}
			// } else {

			// 	model.data[k] = models[i].data[k]
			// }
		}
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	if bToMb(m.Alloc) > 512 { // if memory exceeds 512mb
		model.DumpMicro(model.dirfile + "/")
		references = []string{}
		referencesMap = map[string]bool{}
		model.New(model.dirfile)
		runtime.GC()
	}

}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

func (tempmodel *autocomplete) JustLearn(input string, specificAnswerTemp string) {
	ref := ""

	for t := 0; t <= 15; t++ {
		if len(input) >= t {
			ref = input[:t]
		}
	}
	if ref != "" {
		scoreCount := 1 //len(strings.Split(input[x:], " "))
		if _, ok5 := tempmodel.data[ref]; !ok5 {
			tempmodel.data[ref] = make(map[string]map[string]int)
		}
		if _, ok4 := tempmodel.data[ref][input]; ok4 {

			if _, ok2 := tempmodel.data[ref][input][specificAnswerTemp]; ok2 {

				tempmodel.data[ref][input][specificAnswerTemp] += scoreCount

			} else {

				tempmodel.data[ref][input][specificAnswerTemp] = scoreCount

			}

		} else {

			tempmodel.data[ref][input] = make(map[string]int)
			tempmodel.data[ref][input][specificAnswerTemp] = scoreCount

		}

	}
	//
}

func (model *autocomplete) randomInt(min, max int) int {
	rand.Seed(time.Now().UnixNano())
	return min + rand.Intn(max-min)
}

func (model *autocomplete) Cull() {

	for ref, v1 := range model.data {
		for pref, v2 := range v1 {
			for suff, score := range v2 {
				if float64(score) < (model.total/model.count)-((model.max-(model.total/model.count))*2) {
					model.total = model.total - float64(score)
					model.count = model.count - 1.0
					delete(model.data[ref][pref], suff)

				}
			}
		}
	}
	//	fmt.Println(model.max, "max")
	//	fmt.Println((model.total/model.count)-(model.max-(model.total/model.count)), "min")
	//	fmt.Println(model.total/model.count, "avg")
}

func (model *autocomplete) Load(path string) {

	model.New(model.dirfile)
	dat, err := ioutil.ReadFile(path)

	if err == nil {

		var network bytes.Buffer // Stand-in for a network connection
		//enc := gob.NewEncoder(&network) // Will write to network.
		dec := gob.NewDecoder(&network) // Will read from network.

		//dec := gob.NewDecoder(&network)
		network.Write(dat)
		mut.Lock()
		err = dec.Decode(&model.data)
		mut.Unlock()
		if err != nil {
			model.New(model.dirfile)
			model.Save(path)
		}

	} else if err != nil {
		model.New(model.dirfile)
		model.Save(path)

	}
}

func (model *autocomplete) Save(path string) {
	d := model.data
	var network bytes.Buffer        // Stand-in for a network connection
	enc := gob.NewEncoder(&network) // Will write to network.
	mut.Lock()
	err := enc.Encode(model.data)
	mut.Unlock()
	if err == nil {
		ioutil.WriteFile(path, network.Bytes(), 0644)
	} else {
		model.data = d
	}
}

func (model *autocomplete) WriteText(path string, data string, perm string) bool {
	u, e := strconv.ParseUint(perm, 10, 32)

	if e != nil {
		return false
	}

	err := ioutil.WriteFile(path, []byte(data), os.FileMode(u)) // just pass the file name
	if err != nil {
		return true
	}

	return false
}

func (model *autocomplete) ReadText(path string) string {
	b, err := ioutil.ReadFile(path) // just pass the file name
	if err != nil {
		return ""
	}

	return string(b)
}

func (model *autocomplete) IsDir(name string) bool {

	fi, err := os.Stat(name)
	if err != nil {
		// fmt.Println(err)
		return false
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		// do directory stuff
		return true
	case mode.IsRegular():
		// do file stuff
		return false
	}

	return false
}

func (model *autocomplete) Remember(input string, GuessTheshold uint8, searchAll bool) map[string]int {

	ref := ""

	input = strings.Replace(input, "\\|", "\\\\\\\\\\", -1)

	//wordSplitter := ""

	if strings.Contains(input, "|") {
		wordsSplit := strings.Split(input, "|")
		input = wordsSplit[0]
		//wordSplitter = wordsSplit[1]
	}

	words := []string{input}
	//fmt.Println(words)
	for x := 0; x <= len(words); x++ {
		totalScore := 0
		bestScore := 0
		bestPhrase := ""

		input = words[0] //strings.Join(words[x:], wordSplitter)

		//input = strings.TrimSpace(input)
		//fmt.Println(input)
		//fmt.Println(input)
		for t := 0; t <= 15; t++ {
			if len(input) >= t {
				ref = input[:t]
			}
		}
		scoremsi := make(map[string]int)
		if ref != "" {

			hash := model.Hash(ref)
			// fmt.Println(md5, model.data[ref])
			fold := fmt.Sprint(model.HashCompare("", hash))

			// fmt.Println(ref, md5)

			if GuessTheshold > 0 {

				inputhash := model.Hash(input)

				exactFilePath, _ := filepath.Abs(model.dirfile + "/" + fold + "/" + hash)
				exactFilePathDir, _ := filepath.Abs(model.dirfile + "/" + fold)

				// model.LoadMicro(exactFilePath, ref)

				// if _, ok3 := model.data[ref]; ok3 {
				// 	//fmt.Println(model.data[ref])
				// 	if sug, ok := model.data[ref][input]; ok {
				// 		//					fmt.Println(model.data[ref][input])
				// 		for phrase, score := range sug {

				// 			totalScore = score + totalScore

				// 			// if score >= bestScore {

				// 			bestPhrase = phrase
				// 			bestScore = score

				// 			if s, ok := scoremsi[bestPhrase]; ok {
				// 				scoremsi[bestPhrase] = score + s
				// 			} else {
				// 				scoremsi[bestPhrase] = score
				// 			}

				// 			// }

				// 		}

				// 	}
				// }

				// model.Forget(ref)

				if searchAll != true {
					filepath.Walk(exactFilePathDir, func(path string, info os.FileInfo, err error) error {
						if bestPhrase == "" {
							path, _ = filepath.Abs(path)
							if path != exactFilePath {

								//	fmt.Println(path)
								ref := ""

								for t := 0; t <= 15; t++ {
									if len(input) >= t {
										ref = input[:t]
									}
								}

								model.LoadMicro(path, ref)

								if _, ok3 := model.data[ref]; ok3 {
									//fmt.Println(model.data[ref])
									for inp, _ := range model.data[ref] {
										if sug, ok := model.data[ref][inp]; ok {
											//					fmt.Println(model.data[ref][input])

											if model.HashCompare(inputhash, model.Hash(inp)) <= GuessTheshold {

												for phrase, score := range sug {

													totalScore = score + totalScore

													// if score >= bestScore {

													bestPhrase = phrase
													bestScore = score

													if s, ok := scoremsi[bestPhrase]; ok {
														scoremsi[bestPhrase] = score + s
													} else {
														scoremsi[bestPhrase] = score
													}

													// }

												}
											}

										}
									}
								}
								model.Forget(ref)
							}
						}

						return nil
					})

				}
				// if bestPhrase == "" && searchAll != true {
				// 	filepath.Walk(model.dirfile+"/", func(foldPath string, foldPathInfo os.FileInfo, foldPathErr error) error {

				// 		if bestPhrase == "" {
				// 			foldPath, _ = filepath.Abs(foldPath)
				// 			if foldPath != exactFilePathDir {
				// 				//fmt.Println(foldPath)
				// 				filepath.Walk(foldPath, func(path string, info os.FileInfo, err error) error {
				// 					if bestPhrase == "" {
				// 						path, _ = filepath.Abs(path)
				// 						if path != exactFilePath {

				// 							ref := ""

				// 							for t := 0; t <= 15; t++ {
				// 								if len(input) >= t {
				// 									ref = input[:t]
				// 								}
				// 							}

				// 							//	fmt.Println(path)
				// 							model.LoadMicro(path, ref)

				// 							if _, ok3 := model.data[ref]; ok3 {
				// 								//fmt.Println(model.data[ref])
				// 								for inp, _ := range model.data[ref] {
				// 									if sug, ok := model.data[ref][inp]; ok {
				// 										//					fmt.Println(model.data[ref][input])

				// 										if model.HashCompare(inputhash, model.Hash(inp)) <= GuessTheshold {

				// 											for phrase, score := range sug {

				// 												totalScore = score + totalScore

				// 												// if score >= bestScore {

				// 												bestPhrase = phrase
				// 												bestScore = score

				// 												if s, ok := scoremsi[bestPhrase]; ok {
				// 													scoremsi[bestPhrase] = score + s
				// 												} else {
				// 													scoremsi[bestPhrase] = score
				// 												}

				// 												// }

				// 											}
				// 										}

				// 									}
				// 								}
				// 							}

				// 							model.Forget(ref)
				// 						}

				// 					}
				// 					return nil
				// 				})
				// 			}

				// 		}
				// 		return nil
				// 	})

				// }

				if searchAll == true {
					filepath.Walk(model.dirfile+"/", func(foldPath string, foldPathInfo os.FileInfo, foldPathErr error) error {

						//fmt.Println(foldPath)
						filepath.Walk(foldPath, func(path string, info os.FileInfo, err error) error {

							ref := ""

							for t := 0; t <= 15; t++ {
								if len(input) >= t {
									ref = input[:t]
								}
							}

							//	fmt.Println(path)
							model.LoadMicro(path, ref)

							if _, ok3 := model.data[ref]; ok3 {
								//fmt.Println(model.data[ref])
								for inp, _ := range model.data[ref] {
									if sug, ok := model.data[ref][inp]; ok {
										//					fmt.Println(model.data[ref][input])

										if model.HashCompare(inputhash, model.Hash(inp)) <= GuessTheshold {

											for phrase, score := range sug {

												totalScore = score + totalScore

												// if score >= bestScore {

												bestPhrase = phrase
												bestScore = score

												if s, ok := scoremsi[bestPhrase]; ok {
													scoremsi[bestPhrase] = score + s
												} else {
													scoremsi[bestPhrase] = score
												}

												// }

											}
										}

									}
								}
							}

							model.Forget(ref)

							return nil
						})

						return nil
					})

				}

			} else {

				model.LoadMicro(model.dirfile+"/"+fold+"/"+hash, ref)

				if _, ok3 := model.data[ref]; ok3 {
					//fmt.Println(model.data[ref])
					if sug, ok := model.data[ref][input]; ok {
						//					fmt.Println(model.data[ref][input])
						for phrase, score := range sug {

							totalScore = score + totalScore

							// if score >= bestScore {

							bestPhrase = phrase
							bestScore = score

							if s, ok := scoremsi[bestPhrase]; ok {
								scoremsi[bestPhrase] = score + s
							} else {
								scoremsi[bestPhrase] = score
							}

							// }

						}

					}
				}

			}

			model.Forget(ref)
		}
		if bestPhrase != "" {
			//fmt.Println(bestPhrase)
			return scoremsi
		}
	}
	return map[string]int{}
}

var model8080 *autocomplete = new(autocomplete)
var model8081 *autocomplete = new(autocomplete)
var timeStamp int = int(time.Now().Unix())

// func enableCors(w *http.ResponseWriter) {
// 	(*w).Header().Set("Access-Control-Allow-Origin", "*")
// }

func sayHello(ctx *fasthttp.RequestCtx) {

	var model *autocomplete
	if string(ctx.Host()) == "localhost:8080" {

		model = model8080
	} else {

		model = model8081
	}

	ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
	//enableCors(&w)

	for wait {
		time.Sleep(time.Millisecond)
	}

	wait = true

	message := string(ctx.Path())
	message = strings.TrimPrefix(message, "/")

	if model.data == nil {
		if model.IsDir(model.dirfile) {

		} else {
			model.Load(model.dirfile)
		}
	}
	// This expands the distance searched, but costs more resources (memory and time).
	// For spell checking, "2" is typically enough, for query suggestions this can be higher
	btext := message
	if len(btext) >= 3 && btext[:2] == "={" && btext[len(btext)-1:] == "}" {
		// to train word ={word}
		SpecificAnswer := ""

		words := btext[2 : len(btext)-1]
		wordBreak := " "

		words = strings.Replace(words, "\\|", "\\\\\\\\\\", -1)

		if strings.Contains(words, "|") {
			//={search|||answer}
			w := strings.Split(words, "|")

			SpecificAnswer = strings.Replace(w[1], "\\\\\\\\\\", "|", -1)
			words = strings.Replace(w[0], "\\\\\\\\\\", "|", -1)

			if len(w) >= 3 {
				wordBreak = strings.Replace(w[2], "\\\\\\\\\\", "|", -1)
			}

		}

		if words == "" {
			if model.IsDir(model.dirfile) {

			} else {
				timeStamp = int(time.Now().Unix())
				model.Cull()
				model.Save(model.dirfile)
			}
		} else {
			wordsArray := strings.Split(words, wordBreak)
			SpecificAnswerArray := make([]string, len(wordsArray))
			for x := 0; x < len(SpecificAnswerArray); x++ {
				SpecificAnswerArray[x] = SpecificAnswer
			}
			model.Learn(wordsArray, SpecificAnswerArray)

			//			for y := 0; y < len(words)+1; y += 50 {
			//				if y < len(words)+1 {
			//					//fmt.Println(words[y:])
			//					if model.IsDir(os.Args[1]) {

			//						model.Learn(words[y:], os.Args[1]+"/", SpecificAnswer)
			//					} else {
			//						model.JustLearn(words[y:])
			//					}
			//				}

			//			}
			trainingSessions = trainingSessions + 1
			// if model.max > 1000 {
			// 	if trainingSessions+1%int(model.max*model.max)+1 == 0 {
			// 		model.Cull()
			// 	}
			// }
		}
		//if int(time.Now().Unix())-timeStamp > 30 {
		//	timeStamp = int(time.Now().Unix())
		/*go*/

		//	}
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		if bToMb(m.Alloc) > 30 { // if memory exceeds 30mb
			model.DumpMicro(model.dirfile + "/")
			references = []string{}
			referencesMap = map[string]bool{}
			model.New(model.dirfile)
			runtime.GC()
		} else {
			//fmt.Println(model.data)
			model.DumpMicro(model.dirfile + "/")
		}

		ctx.SetBody([]byte(""))

	} else if len(btext) >= 3 && btext[:2] == "!{" && btext[len(btext)-1:] == "}" {
		words := btext[2 : len(btext)-1]

		strings.Replace(words, "\\|", "\\\\\\\\\\", -1)

		inpArr := strings.Split(words, "|")

		for x := 0; x < len(inpArr); x++ {
			inpArr[x] = strings.Replace(inpArr[x], "\\\\\\\\\\", "|", -1)
		}

		if words == "" {
			if model.IsDir(model.dirfile) {

			} else {
				timeStamp = int(time.Now().Unix())
				model.Save(model.dirfile)
			}

		} else {

			if len(inpArr) == 2 {
				if model.IsDir(model.dirfile) {
					ref := ""

					for t := 0; t <= 15; t++ {
						if len(inpArr[0]) >= t {
							ref = inpArr[0][:t]
						}
					}

					hash := model.Hash(ref)
					// fmt.Println(md5, model.data[ref])
					fold := fmt.Sprint(model.HashCompare("", hash))
					err := os.MkdirAll(model.dirfile+"/"+fold+"/"+hash, 0755)

					//					md5 := model.Md5(ref)
					//					err := os.MkdirAll(os.Args[1]+"/"+md5[:3]+"/"+md5[3:7]+"/"+md5[7:], 0755)
					if err == nil {
						model.LoadMicro(model.dirfile+"/"+fold+"/"+hash, ref)
						model.Punish(inpArr[0], inpArr[1])
						model.SaveMicro(model.dirfile+"/"+fold+"/"+hash, ref)
						model.Forget(ref)
					}
				} else {
					model.Punish(inpArr[0], inpArr[1])
				}

			}
		}

	} else {
		//w.Write([]byte(message))

		//		ref := ""

		//		for t := 0; t <= 15; t++ {
		//			if len(btext) > t {
		//				ref = btext[:t]

		//			}
		//		}

		suggested := stringify(model.Remember(btext, 0, false))
		// fmt.Println(btext, os.Args[1]+"/"+md5[:3]+"/"+md5[3:7]+"/"+md5[7:])
		//		model.Forget(ref)

		fmt.Println(suggested)
		ctx.SetBody([]byte(suggested))

	}

	wait = false

}

var cmd string = ""
var vm *goja.Runtime = goja.New()

// Get the bi-dimensional pixel array
func getPixels(file io.Reader) ([][]Pixel, error) {
	img, _, err := image.Decode(file)

	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y

	var pixels [][]Pixel
	for y := 0; y < height; y++ {
		var row []Pixel
		for x := 0; x < width; x++ {
			row = append(row, rgbaToPixel(img.At(x, y).RGBA()))
		}
		pixels = append(pixels, row)
	}

	return pixels, nil
}

// img.At(x, y).RGBA() returns four uint32 values; we want a Pixel
func rgbaToPixel(r uint32, g uint32, b uint32, a uint32) Pixel {
	return Pixel{int(r / 257), int(g / 257), int(b / 257), int(a / 257)}
}

// Pixel struct example
type Pixel struct {
	R int
	G int
	B int
	A int
}

func main() {

	registeredJSStructs = make(map[string]interface{})
	registeredJSMethods = make(map[string]interface{})

	//	registeredJSStructs["colly.Collector"] = colly.Collector{}
	//	registeredJSStructs["colly.Context"] = colly.Context{}

	registeredJSMethods["go"] = func(i ...interface{}) {
		fmt.Println(reflect.TypeOf(i))
		fmt.Println(reflect.TypeOf(i[0]))
		if len(i) > 0 {
			if m, ok := i[0].(*goja.Object); ok {
				var vm2 *goja.Runtime = &*vm

				keys, _ := vm.RunString("JSON.stringify(Object.keys(this))")

				keyStrings := jsonparse(keys.String()).([]interface{})

				for x := 0; x < len(keyStrings); x++ {

					vm2.Set(""+cast.ToString(keyStrings[x])+"", vm.Get(cast.ToString(keyStrings[x])))
				}

				args := "["
				for x := 1; x < len(i); x++ {

					if x < len(i)-1 {
						args = args + fmt.Sprint(i[x]) + ","
					} else {
						args = args + fmt.Sprint(i[x])
					}
				}
				args = args + "]"
				wg := new(sync.WaitGroup)
				wg.Add(1)
				go func() {
					defer wg.Done()
					vm2.RunString("(" + fmt.Sprint(m) + "(" + args + "))(this)")
					vm2.RunString("this.global = this;")
					vm2.RunString("this.globalkeys = Object.keys(global);")
					vm.Set("global", vm.Get("global"))

					keys, _ := vm2.RunString("JSON.stringify(Object.keys(this))")

					keyStrings := jsonparse(keys.String()).([]interface{})

					for x := 0; x < len(keyStrings); x++ {

						vm.Set(""+cast.ToString(keyStrings[x])+"", vm2.Get(cast.ToString(keyStrings[x])))
					}

					// 	vm.RunString(`
					// for (var globalajioajsdoidajio = 0; globalajioajsdoidajio < globalkeys.length; globalajioajsdoidajio++) {
					// 	 if (globalkeys[globalajioajsdoidajio] != "global") {

					// 		this[globalkeys[globalajioajsdoidajio]] = global[globalkeys[globalajioajsdoidajio]]
					// 	 } else {

					// 	 }
					// }`)

				}()

				wg.Wait()
			} else if m, ok := i[0].(func()); ok {

				//go m()

			} else if m, ok := i[0].(func(...interface{})); ok && len(i) > 1 {
				//fmt.Println("go 2")
				//go m(i[1:]...)
			}
		}

	}

	registeredJSMethods["goja.New"] = goja.New
	registeredJSMethods["colly.NewCollector"] = colly.NewCollector
	registeredJSMethods["http.Get"] = http.Get
	registeredJSMethods["ioutil.TempDir"] = ioutil.TempDir
	registeredJSMethods["ioutil.ReadFile"] = ioutil.ReadFile
	registeredJSMethods["ioutil.WriteFile"] = ioutil.WriteFile
	registeredJSMethods["os.Chmod"] = os.Chmod
	registeredJSMethods["os.TempDir"] = os.TempDir
	registeredJSMethods["os.Chown"] = os.Chown
	registeredJSMethods["os.Chdir"] = os.Chdir
	registeredJSMethods["os.MkdirAll"] = os.MkdirAll
	registeredJSMethods["save_image"] = func(img image.Image, destPath string, qualityOrNumColors int) {
		out, err := os.Create(destPath)
		if err != nil {
			log.Fatal(err)
		}
		defer out.Close()

		// write new image to file
		if strings.Contains(strings.ToLower(destPath), ".png") && (strings.Split(strings.ToLower(destPath), ".png")[1]) == "" {
			png.Encode(out, img)
		} else if strings.Contains(strings.ToLower(destPath), ".gif") && (strings.Split(strings.ToLower(destPath), ".gif")[1]) == "" {
			O := new(gif.Options)
			if qualityOrNumColors > 0 {
				O.NumColors = qualityOrNumColors
			}
			gif.Encode(out, img, O)
		} else if strings.Contains(strings.ToLower(destPath), ".bmp") && (strings.Split(strings.ToLower(destPath), ".bmp")[1]) == "" {
			bmp.Encode(out, img)
		} else {
			O := new(jpeg.Options)
			if qualityOrNumColors > 0 {
				O.Quality = qualityOrNumColors
			}
			jpeg.Encode(out, img, O)
		}
	}
	registeredJSMethods["open_image"] = func(srcPath string) image.Image {
		// open "test.jpg"
		file, err := os.Open(srcPath)
		if err != nil {
			log.Fatal(err)
		}
		var img image.Image

		// decode jpeg into image.Image
		if strings.Contains(strings.ToLower(srcPath), ".png") && (strings.Split(strings.ToLower(srcPath), ".png")[1]) == "" {
			img, err = png.Decode(file)
		} else if strings.Contains(strings.ToLower(srcPath), ".gif") && (strings.Split(strings.ToLower(srcPath), ".gif")[1]) == "" {
			img, err = gif.Decode(file)
		} else if strings.Contains(strings.ToLower(srcPath), ".bmp") && (strings.Split(strings.ToLower(srcPath), ".bmp")[1]) == "" {
			img, err = bmp.Decode(file)
		} else {
			img, err = jpeg.Decode(file)
		}
		if err != nil {
			log.Fatal(err)
		}
		file.Close()

		return img

	}
	registeredJSMethods["resize_image"] = func(width uint, height uint, img image.Image) image.Image {
		// open "test.jpg"
		// file, err := os.Open(srcPath)
		// if err != nil {
		// 	log.Fatal(err)
		// }
		// var img image.Image

		// decode jpeg into image.Image
		// if strings.Contains(strings.ToLower(srcPath), ".png") && (strings.Split(strings.ToLower(srcPath), ".png")[1]) == "" {
		// 	img, err = png.Decode(file)
		// } else {
		// 	img, err = jpeg.Decode(file)
		// }
		// if err != nil {
		// 	log.Fatal(err)
		// }
		// file.Close()

		// resize to width 1000 using Lanczos resampling
		// and preserve aspect ratio
		return resize.Resize(width, height, img, resize.Lanczos3)

		// out, err := os.Create(destPath)
		// if err != nil {
		// 	log.Fatal(err)
		// }
		// defer out.Close()

		// // write new image to file
		// if strings.Contains(strings.ToLower(destPath), ".png") && (strings.Split(strings.ToLower(destPath), ".png")[1]) == "" {
		// 	png.Encode(out, m)
		// } else {
		// 	jpeg.Encode(out, m, nil)
		// }
	}

	registeredJSMethods["image_to_array"] = func(img image.Image) [][][]uint8 {
		// Create an 100 x 50 image
		// You can register another format here

		// image.RegisterFormat("png", "png", png.Decode, png.DecodeConfig)

		// file, err := os.Open(path)

		// if err != nil {

		// } else {

		// defer file.Close()

		bounds := img.Bounds()
		width, height := bounds.Max.X, bounds.Max.Y

		var pixels [][]Pixel
		for y := 0; y < height; y++ {
			var row []Pixel
			for x := 0; x < width; x++ {
				row = append(row, rgbaToPixel(img.At(x, y).RGBA()))
			}
			pixels = append(pixels, row)
		}

		arr := [][][]uint8{}
		for y := 0; y < len(pixels); y++ {
			arr = append(arr, [][]uint8{})
			for x := 0; x < len(pixels[y]); x++ {
				arr[y] = append(arr[y], []uint8{})

				arr[y][x] = append(arr[y][x], uint8(pixels[y][x].R))
				arr[y][x] = append(arr[y][x], uint8(pixels[y][x].G))
				arr[y][x] = append(arr[y][x], uint8(pixels[y][x].B))
				arr[y][x] = append(arr[y][x], uint8(pixels[y][x].A))

			}
		}
		return arr

		// }

	}

	registeredJSMethods["array_to_image"] = func(arr [][][]uint8) image.Image {
		// Create an 100 x 50 image
		w := 0
		h := len(arr)

		var img *image.RGBA

		for y := 0; y < len(arr); y++ {
			for x := 0; x < len(arr[y]); x++ {
				if w < x {
					w = x
				}
			}

		}

		img = image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < len(arr); y++ {
			for x := 0; x < len(arr[y]); x++ {

				if len(arr[y][x]) >= 4 {
					img.Set(x, y, color.RGBA{arr[y][x][0], arr[y][x][1], arr[y][x][2], arr[y][x][3]})
				}

			}
		}

		// Save to out.png
		// f, _ := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
		// defer f.Close()
		// if strings.Contains(strings.ToLower(path), ".png") && (strings.Split(strings.ToLower(path), ".png")[1]) == "" {
		// 	png.Encode(f, img)
		// } else {
		// 	jpeg.Encode(f, img, nil)
		// }

		return img.SubImage(image.Rect(0, 0, w, h))

	}
	registeredJSMethods["os.Mkdir"] = os.Mkdir
	// registeredJSMethods["convnet.LSTMNet.LSTMNet"]
	//	registeredJSMethods["convnet.LSTMNet.LSTMNet"] = convnet.LSTMNet{}.
	//	registeredJSMethods["convnet.Trainer.TrainNormalNet"] = convnet.Trainer{}.TrainNormalNet
	//	registeredJSMethods["convnet.LSTMNet.FromJSON"] = convnet.LSTMNet{}.FromJSON
	//	registeredJSMethods["convnet.LSTMNet.ToJSON"] = convnet.LSTMNet{}.ToJSON
	//	registeredJSMethods["convnet.LSTMNet.ForwardLSTM"] = convnet.LSTMNet{}.ForwardLSTM
	registeredJSStructs["quickLook"] = make(map[string]interface{})

	registeredJSStructs["quickLook"].(map[string]interface{})["New"] = func(path string) *autocomplete { a := new(autocomplete); a.New(path); return a }

	registeredJSStructs["os.Args"] = os.Args

	registeredJSMethods["b2s"] = func(b []byte) string {
		return string(b)
	}

	registeredJSMethods["s2b"] = func(s string) []byte {
		return []byte(s)
	}

	vm.Set("Time", php2go.Time)
	vm.Set("Strtotime", php2go.Strtotime)
	vm.Set("Date", php2go.Date)
	vm.Set("Checkdate", php2go.Checkdate)
	vm.Set("Sleep", php2go.Sleep)
	vm.Set("Usleep", php2go.Usleep)
	vm.Set("Strpos", php2go.Strpos)
	vm.Set("Stripos", php2go.Stripos)
	vm.Set("Strrpos", php2go.Strrpos)
	vm.Set("Strripos", php2go.Strripos)
	vm.Set("StrReplace", php2go.StrReplace)
	vm.Set("Strtoupper", php2go.Strtoupper)
	vm.Set("Strtolower", php2go.Strtolower)
	vm.Set("Ucfirst", php2go.Ucfirst)
	vm.Set("Lcfirst", php2go.Lcfirst)
	vm.Set("Ucwords", php2go.Ucwords)
	vm.Set("Substr", php2go.Substr)
	vm.Set("Strrev", php2go.Strrev)
	vm.Set("ParseStr", php2go.ParseStr)
	vm.Set("NumberFormat", php2go.NumberFormat)
	vm.Set("ChunkSplit", php2go.ChunkSplit)
	vm.Set("StrWordCount", php2go.StrWordCount)
	vm.Set("Wordwrap", php2go.Wordwrap)
	vm.Set("Strlen", php2go.Strlen)
	vm.Set("MbStrlen", php2go.MbStrlen)
	vm.Set("StrRepeat", php2go.StrRepeat)
	vm.Set("Strstr", php2go.Strstr)
	vm.Set("Strtr", php2go.Strtr)
	vm.Set("StrShuffle", php2go.StrShuffle)
	vm.Set("Trim", php2go.Trim)
	vm.Set("Ltrim", php2go.Ltrim)
	vm.Set("Rtrim", php2go.Rtrim)
	vm.Set("Explode", php2go.Explode)
	vm.Set("Chr", php2go.Chr)
	vm.Set("Ord", php2go.Ord)
	vm.Set("Nl2br", php2go.Nl2br)
	vm.Set("JsonDecode", php2go.JsonDecode)
	vm.Set("JsonEncode", php2go.JsonEncode)
	vm.Set("Addslashes", php2go.Addslashes)
	vm.Set("Stripslashes", php2go.Stripslashes)
	vm.Set("Quotemeta", php2go.Quotemeta)
	vm.Set("Htmlentities", php2go.Htmlentities)
	vm.Set("HtmlEntityDecode", php2go.HtmlEntityDecode)
	vm.Set("Md5", php2go.Md5)
	vm.Set("Md5File", php2go.Md5File)
	vm.Set("Sha1", php2go.Sha1)
	vm.Set("Sha1File", php2go.Sha1File)
	vm.Set("Crc32", php2go.Crc32)
	vm.Set("Levenshtein", php2go.Levenshtein)
	vm.Set("SimilarText", php2go.SimilarText)
	vm.Set("Soundex", php2go.Soundex)
	vm.Set("ParseUrl", php2go.ParseUrl)
	vm.Set("UrlEncode", php2go.UrlEncode)
	vm.Set("UrlDecode", php2go.UrlDecode)
	vm.Set("Rawurlencode", php2go.Rawurlencode)
	vm.Set("Rawurldecode", php2go.Rawurldecode)
	vm.Set("HttpBuildQuery", php2go.HttpBuildQuery)
	vm.Set("Base64Encode", php2go.Base64Encode)
	vm.Set("Base64Decode", php2go.Base64Decode)
	vm.Set("ArrayFill", php2go.ArrayFill)
	vm.Set("ArrayFlip", php2go.ArrayFlip)
	vm.Set("ArrayKeys", php2go.ArrayKeys)
	vm.Set("ArrayValues", php2go.ArrayValues)
	vm.Set("ArrayMerge", php2go.ArrayMerge)
	vm.Set("ArrayChunk", php2go.ArrayChunk)
	vm.Set("ArrayPad", php2go.ArrayPad)
	vm.Set("ArraySlice", php2go.ArraySlice)
	vm.Set("ArrayRand", php2go.ArrayRand)
	vm.Set("ArrayColumn", php2go.ArrayColumn)
	vm.Set("ArrayPush", php2go.ArrayPush)
	vm.Set("ArrayPop", php2go.ArrayPop)
	vm.Set("ArrayUnshift", php2go.ArrayUnshift)
	vm.Set("ArrayShift", php2go.ArrayShift)
	vm.Set("ArrayKeyExists", php2go.ArrayKeyExists)
	vm.Set("ArrayCombine", php2go.ArrayCombine)
	vm.Set("ArrayReverse", php2go.ArrayReverse)
	vm.Set("Implode", php2go.Implode)
	vm.Set("Abs", php2go.Abs)
	vm.Set("Rand", php2go.Rand)
	vm.Set("Round", php2go.Round)
	vm.Set("Floor", php2go.Floor)
	vm.Set("Ceil", php2go.Ceil)
	vm.Set("Pi", php2go.Pi)
	vm.Set("Max", php2go.Max)
	vm.Set("Min", php2go.Min)
	vm.Set("Decbin", php2go.Decbin)
	vm.Set("Bindec", php2go.Bindec)
	vm.Set("Hex2bin", php2go.Hex2bin)
	vm.Set("Bin2hex", php2go.Bin2hex)
	vm.Set("Dechex", php2go.Dechex)
	vm.Set("Hexdec", php2go.Hexdec)
	vm.Set("Decoct", php2go.Decoct)
	vm.Set("Octdec", php2go.Octdec)
	vm.Set("BaseConvert", php2go.BaseConvert)
	vm.Set("IsNan", php2go.IsNan)
	vm.Set("Stat", php2go.Stat)
	vm.Set("Pathinfo", php2go.Pathinfo)
	vm.Set("FileExists", php2go.FileExists)
	vm.Set("IsFile", php2go.IsFile)
	vm.Set("IsDir", php2go.IsDir)
	vm.Set("FileSize", php2go.FileSize)
	vm.Set("FilePutContents", php2go.FilePutContents)
	vm.Set("FileGetContents", php2go.FileGetContents)
	vm.Set("Unlink", php2go.Unlink)
	vm.Set("Delete", php2go.Delete)
	vm.Set("Copy", php2go.Copy)
	vm.Set("IsReadable", php2go.IsReadable)
	vm.Set("IsWriteable", php2go.IsWriteable)
	vm.Set("Rename", php2go.Rename)
	vm.Set("Touch", php2go.Touch)
	vm.Set("Mkdir", php2go.Mkdir)
	vm.Set("Getcwd", php2go.Getcwd)
	vm.Set("Realpath", php2go.Realpath)
	vm.Set("Basename", php2go.Basename)
	vm.Set("Chmod", php2go.Chmod)
	vm.Set("Chown", php2go.Chown)
	vm.Set("Fclose", php2go.Fclose)
	vm.Set("Filemtime", php2go.Filemtime)
	vm.Set("Fgetcsv", php2go.Fgetcsv)
	vm.Set("Glob", php2go.Glob)
	vm.Set("Empty", php2go.Empty)
	vm.Set("IsNumeric", php2go.IsNumeric)
	vm.Set("Exec", php2go.Exec)
	vm.Set("System", php2go.System)
	vm.Set("Passthru", php2go.Passthru)
	vm.Set("Gethostname", php2go.Gethostname)
	vm.Set("Gethostbyname", php2go.Gethostbyname)
	vm.Set("Gethostbynamel", php2go.Gethostbynamel)
	vm.Set("Gethostbyaddr", php2go.Gethostbyaddr)
	vm.Set("Ip2long", php2go.Ip2long)
	vm.Set("Long2ip", php2go.Long2ip)
	vm.Set("Echo", php2go.Echo)
	vm.Set("Uniqid", php2go.Uniqid)
	vm.Set("Exit", php2go.Exit)
	vm.Set("Die", php2go.Die)
	vm.Set("Getenv", php2go.Getenv)
	vm.Set("Putenv", php2go.Putenv)
	vm.Set("MemoryGetUsage", php2go.MemoryGetUsage)
	vm.Set("VersionCompare", php2go.VersionCompare)
	vm.Set("ZipOpen", php2go.ZipOpen)
	vm.Set("Ternary", php2go.Ternary)

	vm.Set("model", func() *autocomplete {
		return new(autocomplete)
	})

	vm.Set("cwd", func() string {

		return filepath.Dir(os.Args[0])
	})

	fmtmsf := make(map[string]func(...interface{}) []interface{})

	fmtmsf["Print"] = func(input ...interface{}) []interface{} {
		var i []interface{} = []interface{}{}

		i = multiFunc(fmt.Println(input))

		return i
	}

	fmtmsf["Println"] = func(input ...interface{}) []interface{} {
		var i []interface{} = []interface{}{}

		i = multiFunc(fmt.Println(input))

		return i
	}

	vm.Set("fmt", fmtmsf)

	vm.Set("string", func(b []byte) string {
		return cast.ToString(b)
	})

	vm.Set("byte", func(b interface{}) []byte {
		return []byte(cast.ToString(b))
	})

	vm.Set("serve", func(address string, callback func(responseContext *fasthttp.RequestCtx)) {

		fasthttp.ListenAndServe("localhost:8080", callback)
	})

	vm.Set("api", func() string {
		previousPrintReflectors = make(map[string]bool)
		return showAPI()
	})

	vm.Set("resolveURL", func(baseString string, ref string) string {
		u, err := url.Parse(ref)
		if err != nil {
			return ""
		}
		base, err := url.Parse(baseString)
		if err != nil {
			return ""
		}

		return base.ResolveReference(u).String()

	})

	vm.Set("args", os.Args)

	for k, v := range registeredJSStructs {
		deepReflectVMSet(v, k, false)
	}

	for k, v := range registeredJSMethods {
		deepReflectVMSet(v, k, true)
	}

	previousPrintReflectors = make(map[string]bool)
	//	fmt.Println(showAPI())

	b, e := ioutil.ReadFile(os.Args[1])

	if e == nil {
		v, err := vm.RunString(strings.Replace(string(b), strings.Split(string(b), "start-js")[0]+"start-js", "", 1))
		if err != nil {
			fmt.Println(err, v)
		}
	}

	//php2go.FilePutContents("./api complete.js", showAPI(), os.FileMode(755))

	//return
	// .exe [model path] [command] [param]

	//	if len(os.Args) > 2 {

	//		if model8080.IsDir(model8080.dirfile) {
	//			model8080.New(os.Args[1])
	//			model8081.New(os.Args[1])
	//		} else {
	//			model8080.Load(os.Args[1])
	//			model8081.Load(os.Args[1])
	//		}

	//		cmd = os.Args[2]
	//	}
	//	if (cmd == "predict" || cmd == "classify") && len(os.Args) > 2 {

	//		fmt.Println("started")
	//		// http.HandleFunc("/", sayHello)
	//		// if err := http.ListenAndServe("localhost:8080", nil); err != nil {
	//		// 	panic(err)
	//		// }

	//		go fasthttp.ListenAndServe("localhost:8080", sayHello)
	//		go fasthttp.ListenAndServe("localhost:8081", sayHello)

	//		for true {
	//			time.Sleep(time.Millisecond * 10)
	//		}

	//	} else if cmd == "train" && len(os.Args) > 2 {
	//		fmt.Println("training...")
	//		// This expands the distance searched, but costs more resources (memory and time).
	//		// For spell checking, "2" is typically enough, for query suggestions this can be higher
	//		if model8081.IsDir(model8081.dirfile) {

	//			model8081.New(os.Args[1])
	//			path := os.Args[3]

	//			dat, err := ioutil.ReadFile(path)

	//			if err == nil {
	//				sentences := strings.Split(string(dat), ".")

	//				for x := 0; x < len(sentences); x++ {
	//					words := strings.Split(sentences[x]+".", " ")

	//					for y := 0; y < iterations; y++ {
	//						r1 := model8081.randomInt(0, len(words))
	//						r2 := model8081.randomInt(r1, len(words))

	//						if r2-r1 > 5 {
	//							r2 = r1 + 5
	//						}

	//						if r1 < r2 {
	//							// fmt.Println(words[r1:r2])s
	//							model8081.Learn(strings.Join(words[r1:r2], " "), "", " ")
	//						}
	//					}

	//					//nathans
	//					fmt.Println(float64(x) / float64(len(sentences)))
	//				}

	//			}
	//			model8081.DumpMicro(model8081.dirfile + "/")

	//			references = []string{}
	//			referencesMap = map[string]bool{}

	//		} /*else {
	//			model8081.Load(os.Args[1])
	//			path := os.Args[3]

	//			dat, err := ioutil.ReadFile(path)
	//			if err == nil {
	//				sentences := strings.Split(string(dat), ".")

	//				for x := 0; x < len(sentences); x++ {
	//					model8081.JustLearn(sentences[x] + ".")
	//					if x%10000 == 0 {
	//						model8081.Save(os.Args[1])
	//					}
	//				}

	//			}

	//			model8081.Save(os.Args[1])

	//		}*/
	//	} else if len(os.Args) > 2 && os.Args[1] == "js" || len(os.Args) <= 1 {

	//		os.Chdir(filepath.Dir(os.Args[0]))

	//		vm.Set("Time", php2go.Time)
	//		vm.Set("Strtotime", php2go.Strtotime)
	//		vm.Set("Date", php2go.Date)
	//		vm.Set("Checkdate", php2go.Checkdate)
	//		vm.Set("Sleep", php2go.Sleep)
	//		vm.Set("Usleep", php2go.Usleep)
	//		vm.Set("Strpos", php2go.Strpos)
	//		vm.Set("Stripos", php2go.Stripos)
	//		vm.Set("Strrpos", php2go.Strrpos)
	//		vm.Set("Strripos", php2go.Strripos)
	//		vm.Set("StrReplace", php2go.StrReplace)
	//		vm.Set("Strtoupper", php2go.Strtoupper)
	//		vm.Set("Strtolower", php2go.Strtolower)
	//		vm.Set("Ucfirst", php2go.Ucfirst)
	//		vm.Set("Lcfirst", php2go.Lcfirst)
	//		vm.Set("Ucwords", php2go.Ucwords)
	//		vm.Set("Substr", php2go.Substr)
	//		vm.Set("Strrev", php2go.Strrev)
	//		vm.Set("ParseStr", php2go.ParseStr)
	//		vm.Set("NumberFormat", php2go.NumberFormat)
	//		vm.Set("ChunkSplit", php2go.ChunkSplit)
	//		vm.Set("StrWordCount", php2go.StrWordCount)
	//		vm.Set("Wordwrap", php2go.Wordwrap)
	//		vm.Set("Strlen", php2go.Strlen)
	//		vm.Set("MbStrlen", php2go.MbStrlen)
	//		vm.Set("StrRepeat", php2go.StrRepeat)
	//		vm.Set("Strstr", php2go.Strstr)
	//		vm.Set("Strtr", php2go.Strtr)
	//		vm.Set("StrShuffle", php2go.StrShuffle)
	//		vm.Set("Trim", php2go.Trim)
	//		vm.Set("Ltrim", php2go.Ltrim)
	//		vm.Set("Rtrim", php2go.Rtrim)
	//		vm.Set("Explode", php2go.Explode)
	//		vm.Set("Chr", php2go.Chr)
	//		vm.Set("Ord", php2go.Ord)
	//		vm.Set("Nl2br", php2go.Nl2br)
	//		vm.Set("JsonDecode", php2go.JsonDecode)
	//		vm.Set("JsonEncode", php2go.JsonEncode)
	//		vm.Set("Addslashes", php2go.Addslashes)
	//		vm.Set("Stripslashes", php2go.Stripslashes)
	//		vm.Set("Quotemeta", php2go.Quotemeta)
	//		vm.Set("Htmlentities", php2go.Htmlentities)
	//		vm.Set("HtmlEntityDecode", php2go.HtmlEntityDecode)
	//		vm.Set("Md5", php2go.Md5)
	//		vm.Set("Md5File", php2go.Md5File)
	//		vm.Set("Sha1", php2go.Sha1)
	//		vm.Set("Sha1File", php2go.Sha1File)
	//		vm.Set("Crc32", php2go.Crc32)
	//		vm.Set("Levenshtein", php2go.Levenshtein)
	//		vm.Set("SimilarText", php2go.SimilarText)
	//		vm.Set("Soundex", php2go.Soundex)
	//		vm.Set("ParseUrl", php2go.ParseUrl)
	//		vm.Set("UrlEncode", php2go.UrlEncode)
	//		vm.Set("UrlDecode", php2go.UrlDecode)
	//		vm.Set("Rawurlencode", php2go.Rawurlencode)
	//		vm.Set("Rawurldecode", php2go.Rawurldecode)
	//		vm.Set("HttpBuildQuery", php2go.HttpBuildQuery)
	//		vm.Set("Base64Encode", php2go.Base64Encode)
	//		vm.Set("Base64Decode", php2go.Base64Decode)
	//		vm.Set("ArrayFill", php2go.ArrayFill)
	//		vm.Set("ArrayFlip", php2go.ArrayFlip)
	//		vm.Set("ArrayKeys", php2go.ArrayKeys)
	//		vm.Set("ArrayValues", php2go.ArrayValues)
	//		vm.Set("ArrayMerge", php2go.ArrayMerge)
	//		vm.Set("ArrayChunk", php2go.ArrayChunk)
	//		vm.Set("ArrayPad", php2go.ArrayPad)
	//		vm.Set("ArraySlice", php2go.ArraySlice)
	//		vm.Set("ArrayRand", php2go.ArrayRand)
	//		vm.Set("ArrayColumn", php2go.ArrayColumn)
	//		vm.Set("ArrayPush", php2go.ArrayPush)
	//		vm.Set("ArrayPop", php2go.ArrayPop)
	//		vm.Set("ArrayUnshift", php2go.ArrayUnshift)
	//		vm.Set("ArrayShift", php2go.ArrayShift)
	//		vm.Set("ArrayKeyExists", php2go.ArrayKeyExists)
	//		vm.Set("ArrayCombine", php2go.ArrayCombine)
	//		vm.Set("ArrayReverse", php2go.ArrayReverse)
	//		vm.Set("Implode", php2go.Implode)
	//		vm.Set("Abs", php2go.Abs)
	//		vm.Set("Rand", php2go.Rand)
	//		vm.Set("Round", php2go.Round)
	//		vm.Set("Floor", php2go.Floor)
	//		vm.Set("Ceil", php2go.Ceil)
	//		vm.Set("Pi", php2go.Pi)
	//		vm.Set("Max", php2go.Max)
	//		vm.Set("Min", php2go.Min)
	//		vm.Set("Decbin", php2go.Decbin)
	//		vm.Set("Bindec", php2go.Bindec)
	//		vm.Set("Hex2bin", php2go.Hex2bin)
	//		vm.Set("Bin2hex", php2go.Bin2hex)
	//		vm.Set("Dechex", php2go.Dechex)
	//		vm.Set("Hexdec", php2go.Hexdec)
	//		vm.Set("Decoct", php2go.Decoct)
	//		vm.Set("Octdec", php2go.Octdec)
	//		vm.Set("BaseConvert", php2go.BaseConvert)
	//		vm.Set("IsNan", php2go.IsNan)
	//		vm.Set("Stat", php2go.Stat)
	//		vm.Set("Pathinfo", php2go.Pathinfo)
	//		vm.Set("FileExists", php2go.FileExists)
	//		vm.Set("IsFile", php2go.IsFile)
	//		vm.Set("IsDir", php2go.IsDir)
	//		vm.Set("FileSize", php2go.FileSize)
	//		vm.Set("FilePutContents", php2go.FilePutContents)
	//		vm.Set("FileGetContents", php2go.FileGetContents)
	//		vm.Set("Unlink", php2go.Unlink)
	//		vm.Set("Delete", php2go.Delete)
	//		vm.Set("Copy", php2go.Copy)
	//		vm.Set("IsReadable", php2go.IsReadable)
	//		vm.Set("IsWriteable", php2go.IsWriteable)
	//		vm.Set("Rename", php2go.Rename)
	//		vm.Set("Touch", php2go.Touch)
	//		vm.Set("Mkdir", php2go.Mkdir)
	//		vm.Set("Getcwd", php2go.Getcwd)
	//		vm.Set("Realpath", php2go.Realpath)
	//		vm.Set("Basename", php2go.Basename)
	//		vm.Set("Chmod", php2go.Chmod)
	//		vm.Set("Chown", php2go.Chown)
	//		vm.Set("Fclose", php2go.Fclose)
	//		vm.Set("Filemtime", php2go.Filemtime)
	//		vm.Set("Fgetcsv", php2go.Fgetcsv)
	//		vm.Set("Glob", php2go.Glob)
	//		vm.Set("Empty", php2go.Empty)
	//		vm.Set("IsNumeric", php2go.IsNumeric)
	//		vm.Set("Exec", php2go.Exec)
	//		vm.Set("System", php2go.System)
	//		vm.Set("Passthru", php2go.Passthru)
	//		vm.Set("Gethostname", php2go.Gethostname)
	//		vm.Set("Gethostbyname", php2go.Gethostbyname)
	//		vm.Set("Gethostbynamel", php2go.Gethostbynamel)
	//		vm.Set("Gethostbyaddr", php2go.Gethostbyaddr)
	//		vm.Set("Ip2long", php2go.Ip2long)
	//		vm.Set("Long2ip", php2go.Long2ip)
	//		vm.Set("Echo", php2go.Echo)
	//		vm.Set("Uniqid", php2go.Uniqid)
	//		vm.Set("Exit", php2go.Exit)
	//		vm.Set("Die", php2go.Die)
	//		vm.Set("Getenv", php2go.Getenv)
	//		vm.Set("Putenv", php2go.Putenv)
	//		vm.Set("MemoryGetUsage", php2go.MemoryGetUsage)
	//		vm.Set("VersionCompare", php2go.VersionCompare)
	//		vm.Set("ZipOpen", php2go.ZipOpen)
	//		vm.Set("Ternary", php2go.Ternary)
	//		vm.Set("model", func() *autocomplete {
	//			return new(autocomplete)
	//		})

	//		vm.Set("cwd", func() string {

	//			return filepath.Dir(os.Args[0])
	//		})

	//		fmtmsf := make(map[string]func(...interface{}) []interface{})

	//		fmtmsf["Print"] = func(input ...interface{}) []interface{} {
	//			var i []interface{} = []interface{}{}

	//			i = multiFunc(fmt.Println(input))

	//			return i
	//		}

	//		fmtmsf["Println"] = func(input ...interface{}) []interface{} {
	//			var i []interface{} = []interface{}{}

	//			i = multiFunc(fmt.Println(input))

	//			return i
	//		}

	//		vm.Set("fmt", fmtmsf)

	//		vm.Set("newCollector", func() *colly.Collector {
	//			return colly.NewCollector()
	//		})

	//		vm.Set("ctx", func() *fasthttp.RequestCtx {

	//			return new(fasthttp.RequestCtx)

	//		})

	//		vm.Set("string", func(b []byte) string {
	//			return cast.ToString(b)
	//		})

	//		vm.Set("byte", func(b interface{}) []byte {
	//			return []byte(cast.ToString(b))
	//		})

	//		vm.Set("serve", func(address string, callback func(responseContext *fasthttp.RequestCtx)) {

	//			fasthttp.ListenAndServe("localhost:8080", callback)
	//		})

	//		vm.Set("api", func() string {
	//			previousPrintReflectors = make(map[string]bool)
	//			return showAPI()

	//		})

	//		vm.Set("resolveURL", func(baseString string, ref string) string {
	//			u, err := url.Parse(ref)
	//			if err != nil {
	//				return ""
	//			}
	//			base, err := url.Parse(baseString)
	//			if err != nil {
	//				return ""
	//			}

	//			return base.ResolveReference(u).String()

	//		})

	//		vm.Set("args", os.Args)
	//		txt := model8080.ReadText(os.Args[2])

	//		v, err := vm.RunString(strings.Replace(txt, strings.Split(txt, "start-js")[0]+"start-js", "", 1))
	//		if err != nil {
	//			fmt.Println(err, v)
	//		}

	//	}

	//	if len(os.Args) == 2 && os.Args[1] == "api" {
	//		showAPI()
	//	}

}

// C string to Go string
//func C.GoString(*C.char) string

//export Js

var registeredJSStructs map[string]interface{}
var registeredJSMethods map[string]interface{}

func Js(input *C.char) *C.char {

	v, err := vm.RunString(C.GoString(input))
	if err != nil {
		fmt.Println(err, v)
	}

	return C.CString(v.String())
}

func showAPI() string {
	output := ""

	output = output + fmt.Sprintln("function api(){};")
	output = output + fmt.Sprintln("function resolveURL(base_string, ref_string){};")
	output = output + fmt.Sprintln("function Time(){};")
	output = output + fmt.Sprintln("function Strtotime(format, strtime_string){};")
	output = output + fmt.Sprintln("function Date(format_string, timestamp_int64){};")
	output = output + fmt.Sprintln("function Checkdate(month, day, year_int){};")
	output = output + fmt.Sprintln("function Sleep(t_int64){};")
	output = output + fmt.Sprintln("function Usleep(t_int64){};")
	output = output + fmt.Sprintln("function Strpos(haystack, needle_string, offset_int){};")
	output = output + fmt.Sprintln("function Stripos(haystack, needle_string, offset_int){};")
	output = output + fmt.Sprintln("function Strrpos(haystack, needle_string, offset_int){};")
	output = output + fmt.Sprintln("function Strripos(haystack, needle_string, offset_int){};")
	output = output + fmt.Sprintln("function StrReplace(search, replace, subject_string, count_int){};")
	output = output + fmt.Sprintln("function Strtoupper(str_string){};")
	output = output + fmt.Sprintln("function Strtolower(str_string){};")
	output = output + fmt.Sprintln("function Ucfirst(str_string){};")
	output = output + fmt.Sprintln("function Lcfirst(str_string){};")
	output = output + fmt.Sprintln("function Ucwords(str_string){};")
	output = output + fmt.Sprintln("function Substr(str_string, start_uint, length_int){};")
	output = output + fmt.Sprintln("function Strrev(str_string){};")
	output = output + fmt.Sprintln("function ParseStr(encodedString_string){};")
	output = output + fmt.Sprintln("function NumberFormat(number_float64, decimals_uint, decPoint, thousandsSep_string){};")
	output = output + fmt.Sprintln("function ChunkSplit(body_string, chunklen_uint, end_string){};")
	output = output + fmt.Sprintln("function StrWordCount(str_string){};")
	output = output + fmt.Sprintln("function Wordwrap(str_string, width_uint, br_string){};")
	output = output + fmt.Sprintln("function Strlen(str_string){};")
	output = output + fmt.Sprintln("function MbStrlen(str_string){};")
	output = output + fmt.Sprintln("function StrRepeat(input_string, multiplier_int){};")
	output = output + fmt.Sprintln("function Strstr(haystack_string, needle_string){};")
	output = output + fmt.Sprintln("function Strtr(haystack_string, params_interface_){};")
	output = output + fmt.Sprintln("function StrShuffle(str_string){};")
	output = output + fmt.Sprintln("function Trim(str_string, characterMask_string){};")
	output = output + fmt.Sprintln("function Ltrim(str_string, characterMask_string){};")
	output = output + fmt.Sprintln("function Rtrim(str_string, characterMask_string){};")
	output = output + fmt.Sprintln("function Explode(delimiter, str_string){};")
	output = output + fmt.Sprintln("function Chr(ascii_int){};")
	output = output + fmt.Sprintln("function Ord(char_string){};")
	output = output + fmt.Sprintln("function Nl2br(str_string, isXhtml_bool){};")
	output = output + fmt.Sprintln("function JsonDecode(data_byte, val_interface_){};")
	output = output + fmt.Sprintln("function JsonEncode(val_interface_){};")
	output = output + fmt.Sprintln("function Addslashes(str_string){};")
	output = output + fmt.Sprintln("function Stripslashes(str_string){};")
	output = output + fmt.Sprintln("function Quotemeta(str_string){};")
	output = output + fmt.Sprintln("function Htmlentities(str_string){};")
	output = output + fmt.Sprintln("function HtmlEntityDecode(str_string){};")
	output = output + fmt.Sprintln("function Md5(str_string){};")
	output = output + fmt.Sprintln("function Md5File(path_string){};")
	output = output + fmt.Sprintln("function Sha1(str_string){};")
	output = output + fmt.Sprintln("function Sha1File(path_string){};")
	output = output + fmt.Sprintln("function Crc32(str_string){};")
	output = output + fmt.Sprintln("function Levenshtein(str1, str2_string, costIns, costRep, costDel_int){};")
	output = output + fmt.Sprintln("function SimilarText(first, second_string, percent_float64){};")
	output = output + fmt.Sprintln("function Soundex(str_string){};")
	output = output + fmt.Sprintln("function ParseUrl(str_string, component_int){};")
	output = output + fmt.Sprintln("function UrlEncode(str_string){};")
	output = output + fmt.Sprintln("function UrlDecode(str_string){};")
	output = output + fmt.Sprintln("function Rawurlencode(str_string){};")
	output = output + fmt.Sprintln("function Rawurldecode(str_string){};")
	output = output + fmt.Sprintln("function HttpBuildQuery(queryData_url_Values){};")
	output = output + fmt.Sprintln("function Base64Encode(str_string){};")
	output = output + fmt.Sprintln("function Base64Decode(str_string){};")
	output = output + fmt.Sprintln("function ArrayFill(startIndex_int, num_uint, value_interface_){};")
	output = output + fmt.Sprintln("function ArrayFlip(m_map_interface_interface_){};")
	output = output + fmt.Sprintln("function ArrayKeys(elements_map_interface_interface_){};")
	output = output + fmt.Sprintln("function ArrayValues(elements_map_interface_interface_){};")
	output = output + fmt.Sprintln("function ArrayMerge(ss_interface_){};")
	output = output + fmt.Sprintln("function ArrayChunk(s_interface_, size_int){};")
	output = output + fmt.Sprintln("function ArrayPad(s_interface_, size_int, val_interface_){};")
	output = output + fmt.Sprintln("function ArraySlice(s_interface_, offset, length_uint){};")
	output = output + fmt.Sprintln("function ArrayRand(elements_interface_){};")
	output = output + fmt.Sprintln("function ArrayColumn(input_map_string_map_string_interface_, columnKey_string){};")
	output = output + fmt.Sprintln("function ArrayPush(s_interface_, elements_interface_){};")
	output = output + fmt.Sprintln("function ArrayPop(s_interface_){};")
	output = output + fmt.Sprintln("function ArrayUnshift(s_interface_, elements_interface_){};")
	output = output + fmt.Sprintln("function ArrayShift(s_interface_){};")
	output = output + fmt.Sprintln("function ArrayKeyExists(key_interface_, m_map_interface_interface_){};")
	output = output + fmt.Sprintln("function ArrayCombine(s1, s2_interface_){};")
	output = output + fmt.Sprintln("function ArrayReverse(s_interface_){};")
	output = output + fmt.Sprintln("function Implode(glue_string, pieces_string){};")
	output = output + fmt.Sprintln("function Abs(number_float64){};")
	output = output + fmt.Sprintln("function Rand(min, max_int){};")
	output = output + fmt.Sprintln("function Round(value_float64){};")
	output = output + fmt.Sprintln("function Floor(value_float64){};")
	output = output + fmt.Sprintln("function Ceil(value_float64){};")
	output = output + fmt.Sprintln("function Pi(){};")
	output = output + fmt.Sprintln("function Max(nums_float64){};")
	output = output + fmt.Sprintln("function Min(nums_float64){};")
	output = output + fmt.Sprintln("function Decbin(number_int64){};")
	output = output + fmt.Sprintln("function Bindec(str_string){};")
	output = output + fmt.Sprintln("function Hex2bin(data_string){};")
	output = output + fmt.Sprintln("function Bin2hex(str_string){};")
	output = output + fmt.Sprintln("function Dechex(number_int64){};")
	output = output + fmt.Sprintln("function Hexdec(str_string){};")
	output = output + fmt.Sprintln("function Decoct(number_int64){};")
	output = output + fmt.Sprintln("function Octdec(str_string){};")
	output = output + fmt.Sprintln("function BaseConvert(number_string, frombase, tobase_int){};")
	output = output + fmt.Sprintln("function IsNan(val_float64){};")
	output = output + fmt.Sprintln("function Stat(filename_string){};")
	output = output + fmt.Sprintln("function Pathinfo(path_string, options_int){};")
	output = output + fmt.Sprintln("function FileExists(filename_string){};")
	output = output + fmt.Sprintln("function IsFile(filename_string){};")
	output = output + fmt.Sprintln("function IsDir(filename_string){};")
	output = output + fmt.Sprintln("function FileSize(filename_string){};")
	output = output + fmt.Sprintln("function FilePutContents(filename_string, data_string, mode_os_FileMode){};")
	output = output + fmt.Sprintln("function FileGetContents(filename_string){};")
	output = output + fmt.Sprintln("function Unlink(filename_string){};")
	output = output + fmt.Sprintln("function Delete(filename_string){};")
	output = output + fmt.Sprintln("function Copy(source, dest_string){};")
	output = output + fmt.Sprintln("function IsReadable(filename_string){};")
	output = output + fmt.Sprintln("function IsWriteable(filename_string){};")
	output = output + fmt.Sprintln("function Rename(oldname, newname_string){};")
	output = output + fmt.Sprintln("function Touch(filename_string){};")
	output = output + fmt.Sprintln("function Mkdir(filename_string, mode_os_FileMode){};")
	output = output + fmt.Sprintln("function Getcwd(){};")
	output = output + fmt.Sprintln("function Realpath(path_string){};")
	output = output + fmt.Sprintln("function Basename(path_string){};")
	output = output + fmt.Sprintln("function Chmod(filename_string, mode_os_FileMode){};")
	output = output + fmt.Sprintln("function Chown(filename_string, uid, gid_int){};")
	output = output + fmt.Sprintln("function Fclose(handle_os_File){};")
	output = output + fmt.Sprintln("function Filemtime(filename_string){};")
	output = output + fmt.Sprintln("function Fgetcsv(handle_os_File, length_int, delimiter_rune){};")
	output = output + fmt.Sprintln("function Glob(pattern_string){};")
	output = output + fmt.Sprintln("function Empty(val_interface_){};")
	output = output + fmt.Sprintln("function IsNumeric(val_interface_){};")
	output = output + fmt.Sprintln("function Exec(command_string, output_string, returnVar_int){};")
	output = output + fmt.Sprintln("function System(command_string, returnVar_int){};")
	output = output + fmt.Sprintln("function Passthru(command_string, returnVar_int){};")
	output = output + fmt.Sprintln("function Gethostname(){};")
	output = output + fmt.Sprintln("function Gethostbyname(hostname_string){};")
	output = output + fmt.Sprintln("function Gethostbynamel(hostname_string){};")
	output = output + fmt.Sprintln("function Gethostbyaddr(ipAddress_string){};")
	output = output + fmt.Sprintln("function Ip2long(ipAddress_string){};")
	output = output + fmt.Sprintln("function Long2ip(properAddress_uint32){};")
	output = output + fmt.Sprintln("function Echo(args_interface_){};")
	output = output + fmt.Sprintln("function Uniqid(prefix_string){};")
	output = output + fmt.Sprintln("function Exit(status_int){};")
	output = output + fmt.Sprintln("function Die(status_int){};")
	output = output + fmt.Sprintln("function Getenv(varname_string){};")
	output = output + fmt.Sprintln("function Putenv(setting_string){};")
	output = output + fmt.Sprintln("function MemoryGetUsage(realUsage_bool){};")
	output = output + fmt.Sprintln("function VersionCompare(version1, version2, operator_string){};")
	output = output + fmt.Sprintln("function ZipOpen(filename_string){};")
	output = output + fmt.Sprintln("function Ternary(condition_bool, trueVal, falseVal_interface_){};")

	output = output + fmt.Sprintln("var fmt = {};")
	output = output + fmt.Sprintln("fmt.Print = function(_interface_){};")
	output = output + fmt.Sprintln("fmt.Println = function(_interface_){};")
	output = output + fmt.Sprintln("fmt.Print = function(_interface_){};")

	//	deepReflectPrint(http.Client, "http.Client", false, 0, 3)
	//	deepReflectPrint(http.CloseNotifier, false, 0, 3)
	//	deepReflectPrint(http.Cookie, false, 0, 3)
	//	deepReflectPrint(http.CookieJar, false, 0, 3)
	//	output = output + deepReflectPrint(http.Client{}, "http.Client", false, 0, 2)

	for k, v := range registeredJSStructs {
		output = output + deepReflectPrint(v, k, false, 0, 100, []string{})
	}

	for k, v := range registeredJSMethods {
		output = output + deepReflectPrint(v, k, true, 0, 100, []string{})
	}

	_ = html.Node{}
	_ = http.Client{}
	//	output = output + deepReflectPrint(html.Node{}, "html.Node", false, 0, 2)
	//	output = output + deepReflectPrint(html.Token{}, "html.Token", false, 0, 2)
	//	output = output + deepReflectPrint(html.Tokenizer{}, "html.Tokenizer", false, 0, 2)

	//	for _, method := range obj.Fields() {

	//		fmt.Println("http.Client." + method.Name() + " = {}")
	//	}

	//	for _, method := range obj.Methods() {

	//		methodInTypes := fmt.Sprint(method.InTypes())

	//		methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)

	//		fmt.Println("http.Client." + method.Name() + " = function(" + (methodInTypes) + ") {}")
	//	}

	//	fmt.Println("var http.Response = {};")
	//	obj = reflector.New(new(http.Response))

	//	for _, method := range obj.Fields() {

	//		fmt.Println("http.Response." + method.Name() + " = {}")
	//	}

	//	for _, method := range obj.Methods() {

	//		methodInTypes := fmt.Sprint(method.InTypes())

	//		methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)

	//		fmt.Println("http.Response." + method.Name() + " = function(" + (methodInTypes) + ") {}")
	//	}

	//	fmt.Println("var http.Header = {};")
	//	obj = reflector.New(new(http.Header))

	//	for _, method := range obj.Fields() {

	//		fmt.Println("http.Header." + method.Name() + " = {}")
	//	}

	//	for _, method := range obj.Methods() {

	//		methodInTypes := fmt.Sprint(method.InTypes())

	//		methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)

	//		fmt.Println("http.Header." + method.Name() + " = function(" + (methodInTypes) + ") {}")
	//	}

	//	fmt.Println("var http.Header = {};")
	//	obj = reflector.New(new(http.Header))

	//	for _, method := range obj.Fields() {

	//		fmt.Println("http.Header." + method.Name() + " = {}")
	//	}

	//	for _, method := range obj.Methods() {

	//		methodInTypes := fmt.Sprint(method.InTypes())

	//		methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)

	//		fmt.Println("http.Header." + method.Name() + " = function(" + (methodInTypes) + ") {}")
	//	}

	//	fmt.Println("var newCollector = function(){};")

	//	obj = reflector.New(new(colly.Collector))

	//	for _, method := range obj.Methods() {

	//		methodInTypes := fmt.Sprint(method.InTypes())

	//		methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)

	//		fmt.Println("newCollector." + method.Name() + " = function(" + (methodInTypes) + ") {}")
	//	}

	//	fmt.Println("var HTMLElement = {};")

	//	obj = reflector.New(new(colly.HTMLElement))

	//	for _, method := range obj.Methods() {

	//		methodInTypes := fmt.Sprint(method.InTypes())

	//		methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)

	//		fmt.Println("HTMLElement." + method.Name() + " = function(" + (methodInTypes) + ") {}")
	//	}

	//	fmt.Println("HTMLElement.DOM = {};")

	//	obj = reflector.New(new(colly.HTMLElement).DOM)

	//	for _, method := range obj.Methods() {

	//		methodInTypes := fmt.Sprint(method.InTypes())

	//		methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)

	//		fmt.Println("HTMLElement.DOM." + method.Name() + " = function(" + (methodInTypes) + ") {}")
	//	}

	//	fmt.Println("HTMLElement.Response = {};")

	//	obj = reflector.New(new(colly.HTMLElement).Response)

	//	for _, method := range obj.Methods() {

	//		methodInTypes := fmt.Sprint(method.InTypes())

	//		methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)

	//		fmt.Println("HTMLElement.Response." + method.Name() + " = function(" + (methodInTypes) + ") {}")
	//	}

	//	fmt.Println("HTMLElement.DOM.Nodes = {};")

	//	obj = reflector.New(new(html.Node))

	//	for _, method := range obj.Methods() {

	//		methodInTypes := fmt.Sprint(method.InTypes())

	//		methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)

	//		fmt.Println("HTMLElement.DOM.Nodes." + method.Name() + " = function(" + (methodInTypes) + ") {}")
	//	}

	//	fmt.Println("HTMLElement.DOM.Nodes = {};")

	//	obj = reflector.New(new(html.Node))

	//	for _, method := range obj.Fields() {

	//		fmt.Println("HTMLElement.DOM.Nodes." + method.Name() + " = {}")
	//	}

	//	fmt.Println("var ctx = {};")

	//	obj = reflector.New(new(fasthttp.RequestCtx))

	//	for _, method := range obj.Fields() {

	//		fmt.Println("ctx." + method.Name() + " = {}")
	//	}

	//	obj = reflector.New(new(fasthttp.RequestCtx))

	//	for _, method := range obj.Methods() {

	//		methodInTypes := fmt.Sprint(method.InTypes())

	//		methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)

	//		fmt.Println("ctx." + method.Name() + " = function(" + (methodInTypes) + ") {}")
	//	}

	//	fmt.Println("var serve = function(_string, fasthttp_RequestCtx_){};")

	//	fmt.Println("var model = function(){};")

	//	obj = reflector.New(new(autocomplete))

	//	for _, method := range obj.Methods() {

	//		methodInTypes := fmt.Sprint(method.InTypes())

	//		methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)

	//		fmt.Println("model." + method.Name() + " = function(" + (methodInTypes) + ") {}")
	//	}

	output = output + fmt.Sprintln("\n// start-js")

	return output
}
func multiFunc(a ...interface{}) []interface{} {
	return a
}

var previousPrintReflectors map[string]bool = make(map[string]bool)

func deepReflectPrint(objInterface interface{}, name string, isFunc bool, depth int, maxDepth int, allPrintReflectors []string) string {
	name = strings.Replace(name, ")", "", -1)
	if strings.Contains(fmt.Sprint(reflect.TypeOf(objInterface)), "reflect.") {
		return ""
	}

	for strings.Contains(fmt.Sprint(reflect.TypeOf(objInterface))[:2], "**") {

		objInterface = reflect.ValueOf(objInterface).Elem().Interface()

	}

	objInterface = reflect.New(reflect.TypeOf(objInterface)).Elem().Interface()

	Names := strings.Split(name, ".")

	//	fmt.Println(reflect.TypeOf(objInterface), isFunc)
	/*if len(Names) >= 2 && Names[len(Names)-1] == Names[len(Names)-2] {
		return ""
	}*/

	if depth >= maxDepth {
		//previousPrintReflectors[name] = true
		return ""
	}

	//	if _, ok := previousPrintReflectors[name]; ok {

	//		return ""
	//	} else {

	//	}

	if depth == 0 && !isFunc {

		if len(Names) == 1 {
			previousPrintReflectors[name] = true
			allPrintReflectors = append(allPrintReflectors, "var "+name+" = {};")
		} else {

			if _, ok := previousPrintReflectors[Names[0]]; !ok {
				previousPrintReflectors[Names[0]] = true
				allPrintReflectors = append(allPrintReflectors, "var "+Names[0]+" = {};")
			}
			if _, ok := previousPrintReflectors[name]; !ok {

				previousPrintReflectors[name] = true
				allPrintReflectors = append(allPrintReflectors, name+" = {};")
			}

		}
		//	fmt.Println(reflect.TypeOf(objInterface))
		//t := reflect.New(reflect.TypeOf(objInterface)).Type()

		obj := reflector.New(objInterface)

		//fmt.Println(reflect.TypeOf(reflect.Indirect(reflect.ValueOf(objInterface)).Interface()))
		for _, method := range obj.Fields() {

			if !strings.Contains(fmt.Sprint(method.Type()), "reflect.") {

				allPrintReflectors = append(allPrintReflectors, name+"."+method.Name()+" = {}")
				mtype := method.Type() // <---- their is somethign wrong here...
				if mtype != nil {
					m := reflect.New(mtype).Interface()
					if strings.Contains(fmt.Sprint(mtype), "*") && strings.Split(strings.Split(fmt.Sprint(mtype), "*")[1], ".")[0] == Names[0] {

						allPrintReflectors = append(allPrintReflectors, strings.TrimSpace(deepReflectPrint(m, strings.Split(fmt.Sprint(mtype), "*")[1], false, 0, maxDepth, []string{})))
					}
				}
			}

		}

		for _, method := range obj.Methods() {

			if !strings.Contains(fmt.Sprint(reflect.TypeOf(method.ObjMethodMetadata.ToMethod())), "reflect.") {

				methodInTypes := fmt.Sprint(method.InTypes())

				methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)
				if _, ok := previousPrintReflectors[name]; !ok {
					allPrintReflectors = append(allPrintReflectors, name+"."+method.Name()+" = function("+(methodInTypes)+") {}")
				}

				mt := method.OutTypes()

				for _, mtype := range mt {
					m := reflect.New(mtype).Interface()
					if _, ok := previousPrintReflectors[fmt.Sprint(reflect.TypeOf(objInterface))]; !ok {

						if strings.Contains(fmt.Sprint(mtype), "*") && (strings.Split(strings.Split(fmt.Sprint(mtype), "*")[1], ".")[0]) == strings.Split(name, ".")[0] {

							allPrintReflectors = append(allPrintReflectors, strings.TrimSpace(deepReflectPrint(m, strings.Split(fmt.Sprint(mtype), "*")[1], mtype.Kind() == reflect.Func, 0, maxDepth, []string{})))
						}
						previousPrintReflectors[fmt.Sprint(reflect.TypeOf(objInterface))] = true
					}

				}

				mt = method.InTypes()

				for _, mtype := range mt {
					if _, ok := previousPrintReflectors[fmt.Sprint(reflect.TypeOf(objInterface))]; !ok {
						m := reflect.New(mtype).Interface()

						if strings.Contains(fmt.Sprint(mtype), "*") && (strings.Split(strings.Split(fmt.Sprint(mtype), "*")[1], ".")[0]) == strings.Split(name, ".")[0] {

							allPrintReflectors = append(allPrintReflectors, strings.TrimSpace(deepReflectPrint(m, strings.Split(fmt.Sprint(mtype), "*")[1], mtype.Kind() == reflect.Func, 0, maxDepth, []string{})))

						}

						previousPrintReflectors[fmt.Sprint(reflect.TypeOf(objInterface))] = true
					}

				}
			}

		}

	} else if depth == 0 && isFunc {

		ty := reflect.TypeOf(objInterface)

		// inTypes are default
		tyNum := ty.NumIn()
		tyFn := ty.In

		in := make([]reflect.Type, tyNum)
		for i := 0; i < tyNum; i++ {
			in[i] = tyFn(i)
		}

		methodInTypes := fmt.Sprint(in)

		methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)

		if _, ok := previousPrintReflectors[Names[0]]; !ok {
			allPrintReflectors = append(allPrintReflectors, "var "+Names[0]+" = {};")
		}
		if _, ok := previousPrintReflectors[name]; !ok {
			allPrintReflectors = append(allPrintReflectors, name+" = function("+(methodInTypes)+") {}")
		}
		if len(Names) == 1 {

		} else {

			previousPrintReflectors[Names[0]] = true

		}

		for _, mtype := range in {

			if strings.Contains(fmt.Sprint(mtype), "*") && (strings.Split(strings.Split(fmt.Sprint(mtype), "*")[1], ".")[0]) == strings.Split(name, ".")[0] {
				m1 := reflect.New(mtype).Interface()

				if mtype.Kind() != reflect.Func {
					allPrintReflectors = append(allPrintReflectors, strings.TrimSpace(deepReflectPrint(m1, strings.Split(fmt.Sprint(mtype), "*")[1], false, 0, maxDepth, []string{})))
				} else {
					m, e := CreateFunc(mtype, doublerReflect)
					if e == nil {
						allPrintReflectors = append(allPrintReflectors, strings.TrimSpace(deepReflectPrint(m, strings.Split(fmt.Sprint(mtype), "*")[1], true, 0, maxDepth, []string{})))

					}
				}

			}

		}

		tyNum = ty.NumOut()

		out := make([]reflect.Type, tyNum)

		tyFn = ty.Out

		for i := 0; i < tyNum; i++ {
			out[i] = tyFn(i)
		}

		for _, mtype := range out {

			if strings.Contains(fmt.Sprint(mtype), "*") && (strings.Split(strings.Split(fmt.Sprint(mtype), "*")[1], ".")[0]) == strings.Split(name, ".")[0] {

				if mtype.Kind() != reflect.Func {

					m := reflect.New(mtype).Interface()

					allPrintReflectors = append(allPrintReflectors, strings.Split(strings.TrimSpace(deepReflectPrint(m, strings.Split(fmt.Sprint(mtype), "*")[1], false, 0, maxDepth, []string{})), "\n")...)
				} else {
					m, e := CreateFunc(mtype, doublerReflect)
					if e == nil {
						allPrintReflectors = append(allPrintReflectors, strings.Split(strings.TrimSpace(deepReflectPrint(m, strings.Split(fmt.Sprint(mtype), "*")[1], true, 0, maxDepth, []string{})), "\n")...)

					}
				}

			}

		}

	}

	//t := reflect.TypeOf(objInterface)

	//	if t == nil {
	//		return ""
	//	}

	//	temp := reflect.New(t).Interface()

	//fmt.Println(reflect.New(t).NumMethod())

	obj := reflector.New(objInterface)

	if isFunc {

	} else {

		for _, method := range obj.Fields() {

			if !strings.Contains(fmt.Sprint(method.Type()), "reflect.") {
				allPrintReflectors = append(allPrintReflectors, name+"."+method.Name()+" = {}")
				mtype := method.Type() // <---- their is somethign wrong here...

				if mtype != nil {

					m := reflect.New(mtype).Interface()

					if strings.Contains(fmt.Sprint(mtype), "*") && (strings.Split(strings.Split(fmt.Sprint(mtype), "*")[1], ".")[0]) == strings.Split(name, ".")[0] {

						if _, ok := previousPrintReflectors[strings.Split(fmt.Sprint(mtype), "*")[1]]; !ok {
							allPrintReflectors = append(allPrintReflectors, strings.TrimSpace(deepReflectPrint(m, strings.Split(fmt.Sprint(mtype), "*")[1], false, 0, maxDepth, []string{})))

						}
					}

				}
			}

		}

		//obj = reflector.NewFromType(t)

		for _, method := range obj.Methods() {

			if strings.Contains(fmt.Sprint(reflect.TypeOf(method.ObjMethodMetadata.ToMethod())), "reflect.") != true {

				methodInTypes := fmt.Sprint(method.InTypes())

				methodInTypes = strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(strings.Replace(methodInTypes, ", ", "_", -1), " ", ", ", -1), "[]", "", -1), "[", "_", -1), "]", "_", -1), ".", "_", -1), ",,", ",", -1), ",,", ",", -1), ",,", ",", -1), "_func", "function", -1), "interface, {}", "interface", -1), "*", "", -1), ")", "_", -1), "(", "_", -1)

				allPrintReflectors = append(allPrintReflectors, name+"."+method.Name()+" = function("+(methodInTypes)+") {}")

				mt := method.OutTypes()

				for _, mtype := range mt {
					if _, ok := previousPrintReflectors[fmt.Sprint(reflect.TypeOf(objInterface))]; !ok {
						m := reflect.New(mtype).Interface()

						if strings.Contains(fmt.Sprint(mtype), "*") && (strings.Split(strings.Split(fmt.Sprint(mtype), "*")[1], ".")[0]) == strings.Split(name, ".")[0] {

							allPrintReflectors = append(allPrintReflectors, strings.TrimSpace(deepReflectPrint(m, strings.Split(fmt.Sprint(mtype), "*")[1], mtype.Kind() == reflect.Func, 0, maxDepth, []string{})))
						}
						previousPrintReflectors[fmt.Sprint(reflect.TypeOf(objInterface))] = true
					}

				}

				mt = method.InTypes()

				for _, mtype := range mt {
					if _, ok := previousPrintReflectors[fmt.Sprint(reflect.TypeOf(objInterface))]; !ok {
						m := reflect.New(mtype).Interface()

						if strings.Contains(fmt.Sprint(mtype), "*") && (strings.Split(strings.Split(fmt.Sprint(mtype), "*")[1], ".")[0]) == strings.Split(name, ".")[0] {

							allPrintReflectors = append(allPrintReflectors, strings.TrimSpace(deepReflectPrint(m, strings.Split(fmt.Sprint(mtype), "*")[1], mtype.Kind() == reflect.Func, 0, maxDepth, []string{})))

						}

					}
					previousPrintReflectors[fmt.Sprint(reflect.TypeOf(objInterface))] = true
				}
			}

		}

	}

	if depth == 0 {
		return strings.Replace(strings.Replace(strings.Join(Unique(allPrintReflectors), "\n")+"\n", "\n\n", "\n", -1), "\n\n", "\n", -1)
		//previousPrintReflectors = make(map[string]bool)
	}
	//previousPrintReflectors[name] = true
	return ""

}

func Unique(intSlice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range intSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func CreateFunc(fType reflect.Type, f func(args []reflect.Value) (results []reflect.Value)) (reflect.Value, error) {
	if fType.Kind() != reflect.Func {
		return reflect.Value{}, errors.New("invalid input")
	}

	var ins, outs *[]reflect.Type

	ins = new([]reflect.Type)
	outs = new([]reflect.Type)

	for i := 0; i < fType.NumIn(); i++ {
		*ins = append(*ins, fType.In(i))
	}

	for i := 0; i < fType.NumOut(); i++ {
		*outs = append(*outs, fType.Out(i))
	}
	var variadic bool
	variadic = fType.IsVariadic()
	return AllocateStackFrame(*ins, *outs, variadic, f), nil
}

func AllocateStackFrame(ins []reflect.Type, outs []reflect.Type, variadic bool, f func(args []reflect.Value) (results []reflect.Value)) reflect.Value {
	var funcType reflect.Type
	funcType = reflect.FuncOf(ins, outs, variadic)
	return reflect.MakeFunc(funcType, f)
}

func doubler(input int) int {
	return input * 2
}

func doublerReflect(args []reflect.Value) (result []reflect.Value) {
	if len(args) != 1 {
		panic(fmt.Sprintf("expected 1 arg, found %d", len(args)))
	}
	if args[0].Kind() != reflect.Int {
		panic(fmt.Sprintf("expected 1 arg of kind int, found 1 args of kind", args[0].Kind()))
	}

	var intVal int64
	intVal = args[0].Int()

	var doubleIntVal int
	doubleIntVal = doubler(int(intVal))

	var returnValue reflect.Value
	returnValue = reflect.ValueOf(doubleIntVal)

	return []reflect.Value{returnValue}
}

var previousSetReflectors map[string]map[string]interface{} = make(map[string]map[string]interface{})

func deepReflectVMSet(objInterface interface{}, name string, isFunc bool) {

	Names := strings.Split(name, ".")

	if len(Names) >= 2 && Names[len(Names)-1] == Names[len(Names)-2] {
		return
	}

	if _, ok := previousPrintReflectors[name]; ok {
		return
	} else {
		//	previousPrintReflectors[name] = true
	}

	if len(Names) == 1 {
		previousPrintReflectors[name] = true
		vm.Set(name, objInterface)

	} else {

		if _, ok := previousPrintReflectors[Names[0]]; !ok {
			previousSetReflectors[Names[0]] = make(map[string]interface{})
			vm.Set(Names[0], previousSetReflectors[Names[0]])
		}

		previousPrintReflectors[Names[0]] = true
		previousPrintReflectors[name] = true

		previousSetReflectors[Names[0]][strings.Replace(name, Names[0]+".", "", 1)] = objInterface
		//		vm.Set(name, objInterface)
	}

}
