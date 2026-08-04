package checker

import "testing"

// Tests for structs, methods, and interfaces.
//
// These cases pin the static contract: field access, method dispatch,
// interface satisfaction, and the friendly errors the checker produces
// before a program runs.

func TestCleanStructPrograms(t *testing.T) {
	cases := []string{
		`struct Point { x y }
let p = Point(1, 2)
print(p.x)`,
		`struct Point { x y }
let p = Point(x: 1, y: 2)
print(p.y)`,
		`struct Point { x y }
fn Point.sum() { return self.x + self.y }
let p = Point(1, 2)
print(p.sum())`,
		`struct Point { x y }
let p = Point(1, 2)
p.x = 9
print(p)`,
		`struct Circle { radius }
interface Shape { area() }
fn Circle.area() { return 3.14 * self.radius * self.radius }
let s: Shape = Circle(radius: 2)
print(s.area())`,
		`struct Circle { radius }
interface Shape { area() }
fn Circle.area() { return 0 }
fn draw(s: Shape) { return s.area() }
draw(Circle(radius: 1))`,
		`struct Point { x y }
let p: Point = Point(1, 2)
print(p.x)`,
		"struct Empty {}",
		"interface Contract { run() stop() }",
	}
	for _, src := range cases {
		expectClean(t, src)
	}
}

func TestStructCheckerErrors(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`struct Point { x x }`, "duplicate field"},
		{`struct Point { self }`, "reserved"},
		{`struct Point { x }
fn Point.area() { return 0 }
fn Point.area() { return 1 }`, "duplicate method"},
		{`struct Point { area }
fn Point.area() { return 0 }`, "conflicts with a field"},
		{`fn Point.area() { return 0 }`, "unknown struct type 'Point'"},
		{`interface Shape { area() }
fn Shape.area() { return 0 }`, "cannot add a method to interface"},
		{`struct Point { x }
fn Point.sum() { return self.missing }`, "type 'Point' has no field or method 'missing'"},
		{`struct Point { x }
let p = Point(1)
p.extra = 1`, "no field 'extra'"},
		{`struct Point { x }
let p = Point(1)
p.sum = 1`, "no field 'sum'"},
		{`struct Point { x y }
let p = Point(1, 2, 3)`, "has 2 fields, got 3"},
		{`struct Point { x y }
let p = Point(w: 1, y: 2)`, "has no field 'w'"},
		{`struct Point { x y }
let p = Point(x: 1, x: 2)`, "duplicate field 'x'"},
		{`struct Point { x }
fn f(p: Point) { return p.missing }`, "no field or method 'missing'"},
		{`struct Circle { radius }
interface Shape { area() }
fn Circle.area() { return 0 }
let s: Shape = Circle(radius: 1)
s.radius = 2`, "cannot assign a field through interface 'Shape'"},
		{`struct Circle { radius }
interface Shape { area() }
let s: Shape = Circle(radius: 1)`, "cannot initialize a value of type 'Shape' with a value of type 'Circle'"},
		{`struct Circle { radius }
interface Shape { area() }
fn Circle.area() { return 0 }
print(Shape)`, "cannot use interface 'Shape' as a value"},
		{`struct Point { x }
fn Point.sum() { return self }`, "no error"},
	}
	for _, c := range cases {
		if c.want == "no error" {
			expectClean(t, c.src)
		} else {
			expectError(t, c.src, c.want)
		}
	}
}

func TestStructTypes(t *testing.T) {
	// A struct type name can annotate a variable and a parameter.
	expectClean(t, `struct Point { x y }
let p: Point = Point(1, 2)
fn get_x(q: Point) { return q.x }
print(get_x(p))`)
	// A later struct cannot be used before its declaration.
	expectError(t, `let p: Point = Point(1, 2)
struct Point { x y }`, "unknown type")
}
