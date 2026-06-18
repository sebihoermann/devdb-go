# Sample file with duplicated functions

def function_a():
    x = 1
    y = 2
    z = x + y
    return z

def function_b():
    x = 1
    y = 2
    z = x + y
    return z

# Use both
print(function_a())
print(function_b())
