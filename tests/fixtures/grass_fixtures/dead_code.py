# Sample file with dead code

def used_function(x):
    """This function is called."""
    return x * 2

def dead_function(x):
    """This function is never called."""
    return x * 3

def another_dead_function():
    """Another unused function."""
    print("I am never called")
    return 42

# Only call used_function
result = used_function(5)
