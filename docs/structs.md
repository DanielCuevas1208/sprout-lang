# Structs, Methods, and Interfaces

This page describes Sprout version 0.4.
It covers struct types, methods, and interfaces.

A struct groups related values into one value.
A method is a function that belongs to a struct type.
An interface lists the methods a struct must provide.

## Structs

Declare a struct with the `struct` keyword.
The body lists the field names.

```sprout
struct Point {
    x
    y
}
```

A field holds any value.
Fields do not have static types.
The name `self` is reserved for methods.

Create an instance by calling the type name.
Use positional values or named values.

```sprout
let a = Point(3, 4)
let b = Point(x: 5, y: 7)
```

Positional values fill fields in declaration order.
Named values must name existing fields.
A missing field holds `nil`.
Extra values are an error.

Read and write a field with a dot.

```sprout
print(a.x)       // 3
a.x = 20
print(a)         // Point{x: 20, y: 4}
```

A struct value prints like its source literal.

## Methods

A method is a function that belongs to a struct type.
Declare it with the type name and a dot.

```sprout
fn Point.sum() {
    return self.x + self.y
}

let p = Point(3, 4)
print(p.sum())   // 7
```

The method body reads the receiver through `self`.
Methods may take parameters.

```sprout
struct Box { value }

fn Box.set(v) {
    self.value = v
}

let b = Box(1)
b.set(9)
print(b.value)   // 9
```

A method is a value.
Read it without a call to store or pass it.

```sprout
let setter = b.set
setter(5)
print(b.value)   // 5
```

A method can run in any scope.
It binds to the struct type visible in that scope.
Declaring a method more than once replaces the old one.

A field and a method cannot share a name.

## Interfaces

An interface lists the methods a struct must provide.

```sprout
interface Shape {
    area()
}
```

Each entry names one method and its parameter count.
A struct satisfies an interface when it declares every method
with the same arity.

```sprout
struct Circle { radius }

fn Circle.area() {
    return 3.14 * self.radius * self.radius
}
```

`Circle` satisfies `Shape` because it declares `area()`.

Use an interface as a type annotation.

```sprout
fn describe(s: Shape) {
    return "area " + str(s.area())
}

print(describe(Circle(radius: 2)))
```

The checker verifies that a struct satisfies its interface.
An interface carries no runtime value.
You cannot use an interface name as a value.
You cannot export an interface.

## Type annotations

Use a struct or interface name as a type annotation.

```sprout
let p: Point = Point(1, 2)
let s: Shape = Circle(radius: 1)
```

A struct literal has its declared type.
An unannotated variable keeps the type of its initializer.
The checker rejects an assignment that cannot fit the type.

```sprout
let bad: Shape = 5      // error: int is not a Shape
```

Member access on a known type is checked before the program runs.

```sprout
print(p.unknown)        // error: type 'Point' has no field or method
```

## Modules

Export a struct with the `export` keyword.
Its methods travel with it.

```sprout
export struct Point { x y }
export fn Point.sum() {
    return self.x + self.y
}
```

An importer constructs instances and calls methods.

```sprout
import "geometry" as g
let p = g.Point(2, 3)
print(p.sum())
```

## Limitation

Interface methods check names and arity only.
They do not check parameter types.
