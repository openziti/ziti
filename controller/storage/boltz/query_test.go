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

package boltz

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/ast"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

var businesses = []string{"AllStuff", "Big Boxes Store", "Cables Galore", "Donut Shop", "Farm Equipment", "Game Snob", "Hotel", "Junk Food"}
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

func (p *testPerson) String() string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("[Person id=%v, first=%v, last=%v, age=%v, i32=%v, i64=%v, createdAt=%v, group=%v, places=%v, numbers=%v",
		p.id, p.firstName, p.lastName, p.age, p.index32, p.index64, p.createdAt, p.group, p.places, p.numbers)
}

type boltTest struct {
	dbTest
	referenceTime time.Time
	placesStore   Store
	peopleStore   Store
}

func (test *boltTest) createTestSchema() {
	err := test.db.Update(func(tx *bbolt.Tx) error {
		businessIndex := 0
		placesBucket := GetOrCreatePath(tx, "application", "places")
		for _, place := range places {
			id := uuid.New().String()
			placeMap[place] = id
			placeBucket := placesBucket.GetOrCreatePath(id)
			placeBucket.SetString("name", place, nil)

			var placeBusinesses []string
			placeBusinesses = append(placeBusinesses, businesses[businessIndex%len(businesses)])
			placeBusinesses = append(placeBusinesses, businesses[(businessIndex+1)%len(businesses)])
			placeBusinesses = append(placeBusinesses, businesses[(businessIndex+2)%len(businesses)])
			businessIndex++

			placeBucket.SetStringList("businesses", placeBusinesses, nil)
		}

		placeIndex := 0

		bucket := GetOrCreatePath(tx, "application", "people")
		for i := 0; i < 100; i++ {
			id := uuid.New()
			serviceBucket := bucket.GetOrCreatePath(id.String())

			firstName := firstNames[i%10]
			lastName := lastNames[i/10]
			serviceBucket.SetString("firstName", firstName, nil)
			serviceBucket.SetString("lastName", lastName, nil)

			serviceBucket.SetInt32("age", int32(i), nil)
			serviceBucket.SetInt32("index32", int32(i), nil)
			serviceBucket.SetInt64("index64", 1000-int64(i*10), nil)

			createTime := test.referenceTime.Add(time.Minute * time.Duration(i))
			serviceBucket.SetTimeP("createdAt", &createTime, nil)

			extBucket := serviceBucket.GetOrCreatePath("edge")
			extBucket.SetInt32("group", int32(i%10), nil)

			tagsBucket := extBucket.GetOrCreatePath("tags")
			tagsBucket.SetInt32("age", int32(i), nil)
			tagsBucket.SetBool("ageIsEven", i%2 == 0, nil)
			tagsBucket.SetString("name", firstNames[i%10], nil)

			var numbers []string
			for j := 0; j < 10; j++ {
				numbers = append(numbers, strconv.Itoa(i*10+j))
			}
			serviceBucket.SetStringList("numbers", numbers, nil)

			var personPlaceIds []string
			placeName := places[placeIndex%len(places)]
			personPlaceIds = append(personPlaceIds, placeMap[placeName])
			placeIndex++
			placeName = places[placeIndex%len(places)]
			personPlaceIds = append(personPlaceIds, placeMap[placeName])
			placeIndex++

			serviceBucket.SetStringList("places", personPlaceIds, nil)
		}
		return bucket.Err
	})
	test.NoError(err)
}

func (test *boltTest) setupScanEntity() {
	placeStoreDef := (&StoreDefinition[Entity]{
		EntityType: "places",
	}).WithBasePath("application")

	test.placesStore = NewBaseStore(*placeStoreDef)
	test.placesStore.AddIdSymbol("id", ast.NodeTypeString)
	test.placesStore.AddSymbol("name", ast.NodeTypeString)
	test.placesStore.AddSetSymbol("businesses", ast.NodeTypeString)

	personStoreDef := (&StoreDefinition[Entity]{
		EntityType: "people",
	}).WithBasePath("application")
	test.peopleStore = NewBaseStore(*personStoreDef)
	test.peopleStore.AddIdSymbol("id", ast.NodeTypeString)
	test.peopleStore.AddSymbolWithKey("personAge", ast.NodeTypeInt64, "age")
	test.peopleStore.AddSymbolWithKey("index", ast.NodeTypeInt64, "index32")
	test.peopleStore.AddSymbol("index64", ast.NodeTypeInt64)
	test.peopleStore.AddSymbol("createdAt", ast.NodeTypeDatetime)
	test.peopleStore.AddSymbol("firstName", ast.NodeTypeString)
	test.peopleStore.AddSymbol("lastName", ast.NodeTypeString)
	test.peopleStore.AddSymbol("group", ast.NodeTypeInt64, "edge")
	test.peopleStore.AddMapSymbol("tags", ast.NodeTypeAnyType, "tags", "edge")

	test.peopleStore.AddSetSymbol("numbers", ast.NodeTypeString)
	test.peopleStore.AddFkSetSymbol("places", test.placesStore)
}

func (test *boltTest) toPersonList(ids []string) []*testPerson {
	var result []*testPerson
	err := test.db.View(func(tx *bbolt.Tx) error {
		for _, id := range ids {
			if person := test.loadPerson(tx, id); person != nil {
				result = append(result, person)
			}
		}
		return nil
	})
	test.NoError(err)
	return result
}

func (test *boltTest) loadPerson(tx *bbolt.Tx, id string) *testPerson {
	bucket := Path(tx, "application", "people", id)
	if bucket == nil {
		return nil
	}
	edgeBucket := bucket.GetBucket("edge")
	if edgeBucket == nil {
		return nil
	}
	return &testPerson{
		id:        id,
		firstName: *bucket.GetString("firstName"),
		lastName:  *bucket.GetString("lastName"),
		age:       *bucket.GetInt32("age"),
		index32:   *bucket.GetInt32("index32"),
		index64:   *bucket.GetInt64("index64"),
		createdAt: *bucket.GetTime("createdAt"),
		group:     *edgeBucket.GetInt32("group"),
		numbers:   bucket.GetStringList("numbers"),
		places:    bucket.GetStringList("places"),
		tags:      edgeBucket.GetMap("tags"),
	}
}

func (test *boltTest) query(queryString string) ([]string, int64) {
	var result []string
	var count int64
	var err error
	err = test.db.View(func(tx *bbolt.Tx) error {
		result, count, err = test.peopleStore.QueryIds(tx, queryString)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		fmt.Printf("err: %+v\n", err)
	}
	test.NoError(err)

	return result, count
}

func TestQuery(t *testing.T) {
	boltTestContext := &boltTest{
		referenceTime: time.Now(),
	}
	boltTestContext.switchTestContext(t)
	boltTestContext.init()
	defer boltTestContext.cleanup()

	boltTestContext.createTestSchema()
	boltTestContext.setupScanEntity()

	tests := &boltQueryTests{boltTest: boltTestContext}

	t.Run("first name", tests.testFirstName)
	t.Run("numbers in", tests.testNumbers)
	t.Run("place name equals", tests.testPlaceName)
	t.Run("place name in", tests.testPlaceNamesIn)
	t.Run("place ids in", tests.testPlaceIdsIn)
	t.Run("business equals", tests.testBusinessEquals)
	t.Run("sorting/paging", tests.testSortPage)
	t.Run("map queries", tests.testMapQueries)
	t.Run("sub queries", tests.testSubQueries)
	t.Run("iterate ids", tests.testIterateIds)
	t.Run("iterate ids paging", tests.testIterateIdsPaging)
	t.Run("iterate seek ids", tests.testIterateIdsScan)
	t.Run("test read isolation", tests.testReadIsolation)
}

type boltQueryTests struct {
	*boltTest
}

func (test *boltQueryTests) testFirstName(t *testing.T) {
	test.switchTestContext(t)
	ids, count := test.query(`firstName = "Alice"`)
	test.Equal(10, len(ids))
	test.Equal(int64(10), count)

	people := test.toPersonList(ids)

	test.Equal(10, len(people))

	var foundNames []string
	for _, person := range people {
		test.Equal("Alice", person.firstName)
		test.True(stringz.Contains(lastNames, person.lastName))
		test.False(stringz.Contains(foundNames, person.lastName))
		foundNames = append(foundNames, person.lastName)
	}
}

func (test *boltQueryTests) testNumbers(t *testing.T) {
	test.switchTestContext(t)
	ids, count := test.query(`anyOf(numbers) in [5, 15, 17, 27]`)
	test.Equal(3, len(ids))
	test.Equal(int64(3), count)

	people := test.toPersonList(ids)

	test.Equal(3, len(people))
}

func (test *boltQueryTests) testPlaceName(t *testing.T) {
	test.switchTestContext(t)

	ids, count := test.query(`anyOf(places.name) = "Alphaville"`)
	test.Equal(40, len(ids))
	test.Equal(int64(40), count)

	people := test.toPersonList(ids)
	test.Equal(40, len(people))
}

func (test *boltQueryTests) testPlaceIdsIn(t *testing.T) {
	test.switchTestContext(t)

	var alphaVilleId string

	err := test.db.View(func(tx *bbolt.Tx) error {
		ids, _, err := test.placesStore.QueryIds(tx, `name = "Alphaville"`)
		if err != nil {
			return err
		}
		if len(ids) != 1 {
			return errors.Errorf("unexpected number of places with name Alphaville: %v", len(ids))
		}
		alphaVilleId = ids[0]
		return nil
	})
	test.NoError(err)

	ids, count := test.query(fmt.Sprintf(`anyOf(places.id) = "%v"`, alphaVilleId))
	test.Equal(40, len(ids))
	test.Equal(int64(40), count)

	people := test.toPersonList(ids)
	test.Equal(40, len(people))
}

func (test *boltQueryTests) testPlaceNamesIn(t *testing.T) {
	test.switchTestContext(t)

	ids, count := test.query(`anyOf(places.name) in ["Alphaville", "Betaville"]`)
	test.Equal(60, len(ids))
	test.Equal(int64(60), count)

	people := test.toPersonList(ids)
	test.Equal(60, len(people))
}

func (test *boltQueryTests) testBusinessEquals(t *testing.T) {
	test.switchTestContext(t)

	ids, count := test.query(`anyOf(places.businesses) = "Big Boxes Store"`)
	test.Equal(60, len(ids))
	test.Equal(int64(60), count)

	people := test.toPersonList(ids)
	test.Equal(60, len(people))
}

func (test *boltQueryTests) testSortPage(t *testing.T) {
	test.switchTestContext(t)

	ids, count := test.query(`firstName in ["Alice", "Bob"] SORT BY lastName desc`)
	test.Equal(20, len(ids))
	test.Equal(int64(20), count)

	people := test.toPersonList(ids)
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

	ids, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] SORT BY lastName desc, firstName limit 10`)
	test.Equal(10, len(ids))
	test.Equal(int64(40), count)

	people = test.toPersonList(ids)
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

	ids, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] limit 10`)
	test.Equal(10, len(ids))
	test.Equal(int64(40), count)
	assertOrderById(t, test.toPersonList(ids), nil)
	prevLastId := ids[9]

	ids, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] SORT BY lastName desc, firstName skip 10 limit 10`)
	test.Equal(10, len(ids))
	test.Equal(int64(40), count)

	people = test.toPersonList(ids)
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

	ids, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] skip 10 limit 10`)
	test.Equal(10, len(ids))
	test.Equal(int64(40), count)
	assertOrderById(t, test.toPersonList(ids), &prevLastId)
	prevLastId = ids[9]

	ids, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] SORT BY lastName desc, firstName skip 20 limit 10`)
	test.Equal(10, len(ids))
	test.Equal(int64(40), count)

	people = test.toPersonList(ids)
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

	ids, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] skip 20 limit 10`)
	test.Equal(10, len(ids))
	test.Equal(int64(40), count)
	assertOrderById(t, test.toPersonList(ids), &prevLastId)
	prevLastId = ids[9]

	ids, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] SORT BY lastName desc, firstName skip 30 limit 10`)
	test.Equal(10, len(ids))
	test.Equal(int64(40), count)

	people = test.toPersonList(ids)
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

	ids, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] skip 30 limit 10`)
	test.Equal(10, len(ids))
	test.Equal(int64(40), count)
	assertOrderById(t, test.toPersonList(ids), &prevLastId)
	prevLastId = ids[9]

	ids, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] SORT BY lastName desc, firstName skip 40 limit 10`)
	test.Equal(0, len(ids))
	test.Equal(int64(40), count)

	people = test.toPersonList(ids)
	test.Equal(0, len(people))

	ids, count = test.query(`firstName in ["Alice", "Bob", "Cecilia", "David"] skip 40 limit 10`)
	test.Equal(0, len(ids))
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

func (test *boltQueryTests) testMapQueries(t *testing.T) {
	test.switchTestContext(t)

	ids, count := test.query(`tags.age >= 90`)
	test.Equal(10, len(ids))
	test.Equal(int64(10), count)

	people := test.toPersonList(ids)
	test.Equal(10, len(people))

	for _, person := range people {
		age := person.tags["age"].(int32)
		test.True(age >= 90)
	}
}

func (test *boltQueryTests) testSubQueries(t *testing.T) {
	test.switchTestContext(t)

	ids, count := test.query(`not isEmpty(from places where name = "Alphaville" and anyOf(businesses) = "Big Boxes Store")`)
	test.Equal(40, len(ids))
	test.Equal(int64(40), count)

	people := test.toPersonList(ids)
	test.Equal(40, len(people))

	alphavilleId := placeMap["Alphaville"]

	for _, person := range people {
		test.True(stringz.Contains(person.places, alphavilleId))
	}
}

func (test *boltQueryTests) testIterateIds(t *testing.T) {
	test.switchTestContext(t)

	ids, count := test.query("true")
	fmt.Printf("%v ids\n", count)

	iterIdMap := map[string]struct{}{}
	err := test.db.View(func(tx *bbolt.Tx) error {
		cursor := test.peopleStore.IterateIds(tx, ast.BoolNodeTrue)
		test.NotNil(cursor)
		for ; cursor.IsValid(); cursor.Next() {
			id := cursor.Current()
			_, found := iterIdMap[string(id)]
			test.False(found)
			iterIdMap[string(id)] = struct{}{}
		}
		return nil
	})
	test.NoError(err)
	test.Equal(int(count), len(iterIdMap))

	for _, key := range ids {
		_, found := iterIdMap[key]
		test.True(found)
		delete(iterIdMap, key)
	}
	test.Equal(0, len(iterIdMap))

}

func (test *boltQueryTests) testIterateIdsPaging(t *testing.T) {
	test.switchTestContext(t)

	ids, _ := test.query("skip 5 limit 5")

	iterIdMap := map[string]struct{}{}
	err := test.db.View(func(tx *bbolt.Tx) error {
		query, err := ast.Parse(test.peopleStore, "skip 5 limit 5")
		if err != nil {
			return err
		}
		cursor := test.peopleStore.IterateIds(tx, query)
		test.NotNil(cursor)
		for ; cursor.IsValid(); cursor.Next() {
			id := cursor.Current()
			_, found := iterIdMap[string(id)]
			test.False(found)
			iterIdMap[string(id)] = struct{}{}
		}
		return nil
	})
	test.NoError(err)
	test.Equal(len(ids), len(iterIdMap))

	for _, key := range ids {
		_, found := iterIdMap[key]
		test.True(found)
		delete(iterIdMap, key)
	}
	test.Equal(0, len(iterIdMap))

}

func (test *boltQueryTests) testIterateIdsScan(t *testing.T) {
	test.switchTestContext(t)

	ids, _ := test.query("true")
	middle := ids[len(ids)/2]

	ids, count := test.query(fmt.Sprintf(`id >= "%v"`, middle))

	iterIdMap := map[string]struct{}{}
	err := test.db.View(func(tx *bbolt.Tx) error {
		cursor := test.peopleStore.IterateIds(tx, ast.BoolNodeTrue)
		test.NotNil(cursor)
		cursor.Seek([]byte(middle))
		for ; cursor.IsValid(); cursor.Next() {
			id := cursor.Current()
			_, found := iterIdMap[string(id)]
			test.False(found)
			iterIdMap[string(id)] = struct{}{}
		}
		return nil
	})
	test.NoError(err)
	test.Equal(int(count), len(iterIdMap))

	for _, key := range ids {
		_, found := iterIdMap[key]
		test.True(found)
		delete(iterIdMap, key)
	}
	test.Equal(0, len(iterIdMap))
}

func (test *boltQueryTests) testReadIsolation(t *testing.T) {
	test.switchTestContext(t)

	err := test.db.Update(func(tx *bbolt.Tx) error {
		_ = tx.DeleteBucket([]byte("read-isolation"))
		bucket, err := tx.CreateBucket([]byte("read-isolation"))
		if err != nil {
			return err
		}
		tb := NewTypedBucket(nil, bucket)
		tb.PutValue([]byte("a"), nil)
		tb.PutValue([]byte("b"), nil)
		tb.PutValue([]byte("c"), nil)
		tb.PutValue([]byte("d"), nil)
		tb.PutValue([]byte("e"), nil)
		return tb.GetError()
	})

	ch := make(chan string, 20)

	go func() {
		err = test.db.View(func(tx *bbolt.Tx) error {
			time.Sleep(50 * time.Millisecond)
			bucket := tx.Bucket([]byte("read-isolation"))
			cursor := bucket.Cursor()
			k, _ := cursor.First()
			for k != nil {
				ch <- string(k)
				fmt.Printf("read: %v\n", string(k))
				time.Sleep(50 * time.Millisecond)
				k, _ = cursor.Next()
			}
			close(ch)
			return nil
		})
	}()

	err = test.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("read-isolation"))
		tb := NewTypedBucket(nil, bucket)
		tb.PutValue([]byte("aa"), nil)
		tb.PutValue([]byte("f"), nil)
		fmt.Println("wrote aa")
		fmt.Println("wrote f")
		return nil
	})
	test.NoError(err)
	time.Sleep(50 * time.Millisecond)

	err = test.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("read-isolation"))
		tb := NewTypedBucket(nil, bucket)
		tb.PutValue([]byte("aaa"), nil)
		tb.PutValue([]byte("g"), nil)
		fmt.Println("wrote aaa")
		fmt.Println("wrote g")
		return nil
	})
	test.NoError(err)
	time.Sleep(50 * time.Millisecond)

	err = test.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("read-isolation"))
		tb := NewTypedBucket(nil, bucket)
		tb.PutValue([]byte("aaaa"), nil)
		tb.PutValue([]byte("h"), nil)
		fmt.Println("wrote aaaa")
		fmt.Println("wrote h")
		return nil
	})
	test.NoError(err)

	var vals []string
	for v := range ch {
		fmt.Println(v)
		vals = append(vals, v)
	}
	test.Equal([]string{"a", "b", "c", "d", "e"}, vals)
}
