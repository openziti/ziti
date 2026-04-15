/*
	Copyright NetFoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package objectz

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openziti/foundation/v2/stringz"
)

var places = []string{"Alphaville", "Betaville", "Camden", "Delhi", "Erie"}
var placeMap = map[string]string{}

var firstNames = []string{"Alice", "Bob", "Cecilia", "David", "Emily", "Frank", "Gail", "Hector", "Iggy", "Julia"}
var lastNames = []string{"Smith", "Johnson", "Williams", "Brown", "Jones", "Miller", "Davis", "Garcia", "Rodriguez", "Wilson"}

type testPerson struct {
	id        string
	firstName string
	lastName  string
	age       int32
	index32   int32
	index64   int64
	createdAt time.Time
	group     int32
	numbers   []string
	places    []string
	tags      map[string]interface{}
}

func TestObjectQuery(t *testing.T) {
	tests := &objectQueryTest{
		referenceTime: time.Now(),
	}
	tests.setupTestObjects()
	tests.switchTestContext(t)

	t.Run("first name", tests.testFirstName)
	t.Run("age", tests.testAge)
	t.Run("createdAt", tests.testCreatedAt)
	t.Run("sorting/paging", tests.testSortPage)
}

type objectQueryTest struct {
	*require.Assertions
	referenceTime time.Time
	peopleStore   *ObjectStore[*testPerson]
	people        map[string]*testPerson
}

func (test *objectQueryTest) switchTestContext(t *testing.T) {
	test.Assertions = require.New(t)
}

func (test *objectQueryTest) setupTestObjects() {
	test.people = map[string]*testPerson{}
	test.peopleStore = NewObjectStore(func() ObjectIterator[*testPerson] {
		return IterateMap(test.people)
	})

	test.peopleStore.AddStringSymbol("id", func(entity *testPerson) *string {
		return &entity.id
	})
	test.peopleStore.AddStringSymbol("firstName", func(entity *testPerson) *string {
		return &entity.firstName
	})
	test.peopleStore.AddStringSymbol("lastName", func(entity *testPerson) *string {
		return &entity.lastName
	})
	test.peopleStore.AddInt64Symbol("age", func(entity *testPerson) *int64 {
		val := int64(entity.age)
		return &val
	})
	test.peopleStore.AddDatetimeSymbol("createdAt", func(entity *testPerson) *time.Time {
		return &entity.createdAt
	})

	placeIndex := 0
	for i := 0; i < 100; i++ {
		firstName := firstNames[i%10]
		lastName := lastNames[i/10]
		createTime := test.referenceTime.Add(time.Minute * time.Duration(i))

		var numbers []string
		for j := 0; j < 10; j++ {
			numbers = append(numbers, strconv.Itoa(i*10+j))
		}

		var personPlaceIds []string
		placeName := places[placeIndex%len(places)]
		personPlaceIds = append(personPlaceIds, placeMap[placeName])
		placeIndex++
		placeName = places[placeIndex%len(places)]
		personPlaceIds = append(personPlaceIds, placeMap[placeName])
		placeIndex++

		person := &testPerson{
			id:        uuid.NewString(),
			firstName: firstName,
			lastName:  lastName,
			age:       int32(i),
			index32:   int32(i),
			index64:   1000 - int64(i*10),
			createdAt: createTime,
			group:     int32(i % 10),
			numbers:   numbers,
			places:    personPlaceIds,
			tags: map[string]interface{}{
				"age":       int32(i),
				"ageIsEven": i%2 == 0,
				"name":      firstNames[i%10],
			},
		}
		test.people[person.id] = person
	}
}

func (test *objectQueryTest) query(queryString string) ([]*testPerson, int64) {
	result, count, err := test.peopleStore.QueryEntities(queryString)
	if err != nil {
		fmt.Printf("err: %+v\n", err)
	}
	test.NoError(err)

	return result, count
}

func (test *objectQueryTest) testFirstName(t *testing.T) {
	test.switchTestContext(t)
	people, count := test.query(`firstName = "Alice"`)
	test.Equal(10, len(people))
	test.Equal(int64(10), count)

	var foundNames []string
	for _, person := range people {
		test.Equal("Alice", person.firstName)
		test.True(stringz.Contains(lastNames, person.lastName))
		test.False(stringz.Contains(foundNames, person.lastName))
		foundNames = append(foundNames, person.lastName)
	}
}

func (test *objectQueryTest) testAge(t *testing.T) {
	test.switchTestContext(t)
	people, count := test.query(`age > 10 and age <= 20`)
	test.Equal(10, len(people))
	test.Equal(int64(10), count)

	for _, person := range people {
		test.True(person.age > 10, "age should be > 10, was %v", person.age)
		test.True(person.age <= 20, "age should be <= 20, was %v", person.age)
	}
}

func (test *objectQueryTest) testCreatedAt(t *testing.T) {
	test.switchTestContext(t)
	people, count := test.query(`true sort by createdAt`)
	test.Equal(100, len(people))
	test.Equal(int64(100), count)

	var prev *testPerson
	for _, person := range people {
		if prev != nil {
			test.True(prev.createdAt.Before(person.createdAt),
				"should be sorted by created at. prev: %v, current: %v", prev.createdAt, person.createdAt)
		}
		prev = person
	}
}

func (test *objectQueryTest) testSortPage(t *testing.T) {
	test.switchTestContext(t)

	people, count := test.query(`firstName in ["Alice", "Bob"] SORT BY lastName desc`)
	test.Equal(20, len(people))
	test.Equal(int64(20), count)

	test.Equal(20, len(people))
	for idx, person := range people {
		if idx == 0 {
			continue
		}
		if people[idx-1].lastName == person.lastName {
			test.True(people[idx-1].id < person.id)
		} else {
			test.True(people[idx-1].lastName > person.lastName)
		}
	}

	people, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] SORT BY lastName desc, firstName limit 10`)
	test.Equal(10, len(people))
	test.Equal(int64(40), count)

	test.Equal(10, len(people))
	test.Equal("Wilson", people[0].lastName)
	test.Equal("Alice", people[0].firstName)
	test.Equal("Wilson", people[1].lastName)
	test.Equal("Bob", people[1].firstName)
	test.Equal("Wilson", people[2].lastName)
	test.Equal("Cecilia", people[2].firstName)
	test.Equal("Wilson", people[3].lastName)
	test.Equal("David", people[3].firstName)
	test.Equal("Williams", people[4].lastName)
	test.Equal("Alice", people[4].firstName)
	test.Equal("Williams", people[5].lastName)
	test.Equal("Bob", people[5].firstName)
	test.Equal("Williams", people[6].lastName)
	test.Equal("Cecilia", people[6].firstName)
	test.Equal("Williams", people[7].lastName)
	test.Equal("David", people[7].firstName)
	test.Equal("Smith", people[8].lastName)
	test.Equal("Alice", people[8].firstName)
	test.Equal("Smith", people[9].lastName)
	test.Equal("Bob", people[9].firstName)

	people, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] limit 10`)
	test.Equal(10, len(people))
	test.Equal(int64(40), count)
	assertOrderById(t, people, nil)
	prevLastId := people[9].id

	people, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] SORT BY lastName desc, firstName skip 10 limit 10`)
	test.Equal(10, len(people))
	test.Equal(int64(40), count)

	test.Equal(10, len(people))
	test.Equal("Smith", people[0].lastName)
	test.Equal("Cecilia", people[0].firstName)
	test.Equal("Smith", people[1].lastName)
	test.Equal("David", people[1].firstName)

	test.Equal("Rodriguez", people[2].lastName)
	test.Equal("Alice", people[2].firstName)
	test.Equal("Rodriguez", people[3].lastName)
	test.Equal("Bob", people[3].firstName)
	test.Equal("Rodriguez", people[4].lastName)
	test.Equal("Cecilia", people[4].firstName)
	test.Equal("Rodriguez", people[5].lastName)
	test.Equal("David", people[5].firstName)

	test.Equal("Miller", people[6].lastName)
	test.Equal("Alice", people[6].firstName)
	test.Equal("Miller", people[7].lastName)
	test.Equal("Bob", people[7].firstName)
	test.Equal("Miller", people[8].lastName)
	test.Equal("Cecilia", people[8].firstName)
	test.Equal("Miller", people[9].lastName)
	test.Equal("David", people[9].firstName)

	people, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] skip 10 limit 10`)
	test.Equal(10, len(people))
	test.Equal(int64(40), count)
	assertOrderById(t, people, &prevLastId)
	prevLastId = people[9].id

	people, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] SORT BY lastName desc, firstName skip 20 limit 10`)
	test.Equal(10, len(people))
	test.Equal(int64(40), count)

	test.Equal(10, len(people))
	test.Equal("Jones", people[0].lastName)
	test.Equal("Alice", people[0].firstName)
	test.Equal("Jones", people[1].lastName)
	test.Equal("Bob", people[1].firstName)
	test.Equal("Jones", people[2].lastName)
	test.Equal("Cecilia", people[2].firstName)
	test.Equal("Jones", people[3].lastName)
	test.Equal("David", people[3].firstName)

	test.Equal("Johnson", people[4].lastName)
	test.Equal("Alice", people[4].firstName)
	test.Equal("Johnson", people[5].lastName)
	test.Equal("Bob", people[5].firstName)
	test.Equal("Johnson", people[6].lastName)
	test.Equal("Cecilia", people[6].firstName)
	test.Equal("Johnson", people[7].lastName)
	test.Equal("David", people[7].firstName)

	test.Equal("Garcia", people[8].lastName)
	test.Equal("Alice", people[8].firstName)
	test.Equal("Garcia", people[9].lastName)
	test.Equal("Bob", people[9].firstName)

	people, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] skip 20 limit 10`)
	test.Equal(10, len(people))
	test.Equal(int64(40), count)
	assertOrderById(t, people, &prevLastId)
	prevLastId = people[9].id

	people, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] SORT BY lastName desc, firstName skip 30 limit 10`)
	test.Equal(10, len(people))
	test.Equal(int64(40), count)

	test.Equal(10, len(people))
	test.Equal("Garcia", people[0].lastName)
	test.Equal("Cecilia", people[0].firstName)
	test.Equal("Garcia", people[1].lastName)
	test.Equal("David", people[1].firstName)

	test.Equal("Davis", people[2].lastName)
	test.Equal("Alice", people[2].firstName)
	test.Equal("Davis", people[3].lastName)
	test.Equal("Bob", people[3].firstName)
	test.Equal("Davis", people[4].lastName)
	test.Equal("Cecilia", people[4].firstName)
	test.Equal("Davis", people[5].lastName)
	test.Equal("David", people[5].firstName)

	test.Equal("Brown", people[6].lastName)
	test.Equal("Alice", people[6].firstName)
	test.Equal("Brown", people[7].lastName)
	test.Equal("Bob", people[7].firstName)
	test.Equal("Brown", people[8].lastName)
	test.Equal("Cecilia", people[8].firstName)
	test.Equal("Brown", people[9].lastName)
	test.Equal("David", people[9].firstName)

	people, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] skip 30 limit 10`)
	test.Equal(10, len(people))
	test.Equal(int64(40), count)
	assertOrderById(t, people, &prevLastId)
	prevLastId = people[9].id

	people, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] SORT BY lastName desc, firstName skip 40 limit 10`)
	test.Equal(0, len(people))
	test.Equal(int64(40), count)

	test.Equal(0, len(people))

	people, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] skip 40 limit 10`)
	test.Equal(0, len(people))
	test.Equal(int64(40), count)
}

func assertOrderById(t *testing.T, people []*testPerson, prevId *string) {
	for idx, person := range people {
		if idx == 0 {
			if prevId != nil {
				require.True(t, *prevId < person.id)
			}
			continue
		}
		require.True(t, people[idx-1].id < person.id)
	}
}
