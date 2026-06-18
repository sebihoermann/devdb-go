# Sample file with inlinable functions

def helper_function(x):
    """This function is called only once."""
    return x + 10

def main():
    """Main function that calls helper once."""
    result = helper_function(5)
    print(result)
    return result

# Call main
main()
