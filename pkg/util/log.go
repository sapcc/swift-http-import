/*******************************************************************************
*
* Copyright 2016-2017 SAP SE
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You should have received a copy of the License along with this
* program. If not, you may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*
*******************************************************************************/

package util

import (
	"log"
	"os"
)

type LogLevel int

const (
	LogFatal LogLevel = iota
	LogError
	LogInfo
	LogDebug
)

var logLevelNames = []string{"FATAL", "ERROR", "INFO", "DEBUG"}

var isDebug = os.Getenv("DEBUG") != ""

//Log writes a log message. LogDebug messages are only written if
//the environment variable `DEBUG` is set.
func Log(level LogLevel, msg string, args ...interface{}) {
	if level == LogDebug && !isDebug {
		return
	}

	if len(args) > 0 {
		log.Printf(logLevelNames[level]+": "+msg+"\n", args...)
	} else {
		log.Println(logLevelNames[level] + ": " + msg)
	}

	if level == LogFatal {
		os.Exit(1)
	}
}
