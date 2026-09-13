// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package db

import (
	"os"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/log"

	"xorm.io/xorm"
	"xorm.io/xorm/names"
)

// CreateTestEngine creates an instance of the db engine which lives in memory
func CreateTestEngine() (engine *xorm.Engine, err error) {

	if x != nil {
		return x, nil
	}

	if os.Getenv("VIKUNJA_TESTS_USE_CONFIG") == "1" {
		config.InitConfig()
		engine, err = CreateDBEngine()
		if err != nil {
			return nil, err
		}
	} else {
		engine, err = xorm.NewEngine("sqlite3", "file::memory:?cache=shared")
		if err != nil {
			return nil, err
		}
	}

	engine.SetMapper(names.GonicMapper{})
	logger := log.NewXormLogger(config.LogEnabled.GetBool(), config.LogDatabase.GetString(), "DEBUG", config.LogFormat.GetString())
	logger.ShowSQL(os.Getenv("TESTS_VERBOSE") == "1")
	engine.SetLogger(logger)
	engine.SetTZLocation(config.GetTimeZone())
	engine.AddHook(writeInvalidationHook{})
	x = engine
	return
}

// InitTestFixtures populates the db with all fixtures from the fixtures folder
func InitTestFixtures(tablenames ...string) (err error) {
	// Create all fixtures
	config.InitDefaultConfig()

	// Sync fixtures
	err = InitFixtures(tablenames...)
	if err != nil {
		log.Fatal(err)
	}

	return nil
}
