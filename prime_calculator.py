from sympy import isprime

def calculate_primes(n):
    primes = []
    num = 2
    while len(primes) < n:
        if isprime(num):
            primes.append(num)
        num += 1
    return primes

if __name__ == '__main__':
    print(calculate_primes(10))