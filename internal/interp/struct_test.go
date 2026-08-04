package interp

import "testing"

// Struct, method, and interface behavior on the tree-walking interpreter.

func TestStructLiterals(t *testing.T) {
	expectOutput(t, `
struct Point { x y }
let a = Point(1, 2)
let b = Point(x: 3, y: 4)
print(a)
print(b)
print(a.x, a.y)
print(b.y)
`, "Point{x: 1, y: 2}\nPoint{x: 3, y: 4}\n1 2\n4\n")

	// Missing fields hold nil.
	expectOutput(t, `
struct Point { x y }
let p = Point(5)
print(p)
print(p.y)
`, "Point{x: 5, y: nil}\nnil\n")
}

func TestStructFieldMutation(t *testing.T) {
	expectOutput(t, `
struct Counter { n }
let c = Counter(0)
c.n = c.n + 1
c.n = c.n + 1
print(c.n)
`, "2\n")
}

func TestMethods(t *testing.T) {
	expectOutput(t, `
struct Point { x y }
fn Point.sum() { return self.x + self.y }
let p = Point(3, 4)
print(p.sum())
print(p.sum() + p.sum())
`, "7\n14\n")

	// A method can be read and stored like any value.
	expectOutput(t, `
struct Box { v }
fn Box.get() { return self.v }
let b = Box(42)
let g = b.get
print(g())
`, "42\n")
}

func TestMethodState(t *testing.T) {
	expectOutput(t, `
struct Tally { total }
fn Tally.add(n) {
    self.total = self.total + n
    return self.total
}
let a = Tally(0)
let b = Tally(0)
print(a.add(1), a.add(1), b.add(5))
`, "1 2 5\n")
}

func TestInterfaceSatisfaction(t *testing.T) {
	expectOutput(t, `
struct Square { side }
struct Circle { radius }
interface Shape { area() }
fn Square.area() { return self.side * self.side }
fn Circle.area() { return 3.14 * self.radius * self.radius }
fn report(s: Shape) {
    return str(s.area())
}
print(report(Square(side: 3)))
print(report(Circle(radius: 2)))
`, "9\n12.56\n")
}

func TestStructInsideFunction(t *testing.T) {
	// The bundler runs each module body inside a loader closure, so structs
	// and methods must be valid inside function bodies.
	expectOutput(t, `
fn build() {
    struct Point { x }
    fn Point.get() { return self.x }
    return Point(7).get()
}
print(build())
`, "7\n")
}

func TestStructErrors(t *testing.T) {
	expectError(t, `
struct Point { x y }
let p = Point(1, 2)
print(p.missing)
`, "no field or method 'missing'")

	expectError(t, `
struct Point { x y }
let p = Point(1, 2)
p.missing = 5
`, "no field 'missing'")

	expectError(t, `
struct Point { x y }
let p = Point(1, 2, 3)
`, "expects at most 2 fields, got 3")

	expectError(t, `
struct Point { x y }
let p = Point(w: 1)
`, "no field 'w'")

	expectError(t, `
print(5.foo)
`, "cannot access a member of a int")

	expectError(t, `
struct Point { x }
let p = Point(1)
p.x = 2
print(p.y)
`, "no field or method 'y'")
}
