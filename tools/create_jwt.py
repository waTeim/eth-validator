#!/usr/bin/env python3


"""
A script to manage a Kubernetes secret for an execution target.

This module generates a 32-byte hexadecimal secret using OpenSSL and creates a Kubernetes
secret with the token name `jwt.hex`. If the secret already exists, it can be optionally
regenerated using the --force flag. This secret is intended for use as an execution target
secret between geth and lighthouse.

Usage:
    ./scriptname.py [--force] [--name SECRET_NAME] [--namespace NAMESPACE]
"""

import argparse
import subprocess
import sys

def generate_secret():
    """
    Generate a 32-byte hexadecimal secret using OpenSSL.

    This function runs the OpenSSL command to produce a 32-byte secret in hexadecimal format.

    Returns:
        str: A 32-byte hexadecimal secret.

    Raises:
        SystemExit: Exits if the OpenSSL command fails.
    """
    try:
        result = subprocess.run(
            ['openssl', 'rand', '-hex', '32'],
            check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE
        )
        secret = result.stdout.decode('utf-8').strip()
        return secret
    except subprocess.CalledProcessError as e:
        print(f"Error generating secret using openssl: {e.stderr.decode('utf-8').strip()}")
        sys.exit(1)
    except Exception as e:
        print(f"Unexpected error generating secret: {e}")
        sys.exit(1)

def check_secret_exists(secret_name, namespace):
    """
    Check if a Kubernetes secret exists.

    This function uses kubectl to determine whether a secret with the specified name
    exists in the given namespace.

    Args:
        secret_name (str): The name of the Kubernetes secret.
        namespace (str): The Kubernetes namespace to check.

    Returns:
        bool: True if the secret exists, False otherwise.

    Raises:
        SystemExit: Exits if there is an error checking the secret.
    """
    try:
        result = subprocess.run(
            ['kubectl', 'get', 'secret', secret_name, '-n', namespace],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE
        )
        return result.returncode == 0
    except Exception as e:
        print(f"Error checking secret existence: {e}")
        sys.exit(1)

def delete_secret(secret_name, namespace):
    """
    Delete the Kubernetes secret.

    This function uses kubectl to delete the specified secret from the given namespace.

    Args:
        secret_name (str): The name of the Kubernetes secret to delete.
        namespace (str): The Kubernetes namespace where the secret resides.

    Raises:
        SystemExit: Exits if there is an error deleting the secret.
    """
    try:
        result = subprocess.run(
            ['kubectl', 'delete', 'secret', secret_name, '-n', namespace],
            check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE
        )
        print(result.stdout.decode('utf-8').strip())
    except subprocess.CalledProcessError as e:
        print(f"Error deleting secret: {e.stderr.decode('utf-8').strip()}")
        sys.exit(1)

def create_secret(secret_name, namespace, secret):
    """
    Create a new Kubernetes secret with the given secret value.

    This function uses kubectl to create a new secret in the specified namespace.
    The secret is stored with a token name `jwt.hex` rather than `jwt`.

    Args:
        secret_name (str): The name of the Kubernetes secret to create.
        namespace (str): The Kubernetes namespace where the secret will be created.
        secret (str): The secret token to store.

    Raises:
        SystemExit: Exits if there is an error creating the secret.
    """
    try:
        result = subprocess.run(
            ['kubectl', 'create', 'secret', 'generic', secret_name,
             '--from-literal=jwt.hex=' + secret, '-n', namespace],
            check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE
        )
        print(result.stdout.decode('utf-8').strip())
    except subprocess.CalledProcessError as e:
        print(f"Error creating secret: {e.stderr.decode('utf-8').strip()}")
        sys.exit(1)

def main():
    """
    Main entry point of the script.

    Parses command-line arguments, checks for the existence of the specified Kubernetes secret,
    optionally forces deletion, generates a new secret, and creates the secret in the specified namespace.
    """
    parser = argparse.ArgumentParser(
        description="Check for the Kubernetes secret 'execution-jwt' and generate one if it doesn't exist. "
                    "The secret is suitable for use as an execution target secret between geth and lighthouse."
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Force regeneration of the secret even if it already exists."
    )
    parser.add_argument(
        "--name",
        type=str,
        default="eth-validator-auth-jwt",
        help="Name of the Kubernetes secret (default: eth-validator-auth-jwt )."
    )
    parser.add_argument(
        "--namespace",
        type=str,
        default="default",
        help="Kubernetes namespace to check/create the secret (default: default)."
    )
    
    args = parser.parse_args()

    exists = check_secret_exists(args.name, args.namespace)
    if exists and not args.force:
        print(f"Secret '{args.name}' already exists in namespace '{args.namespace}'. Use --force to regenerate it.")
        sys.exit(0)
    elif exists and args.force:
        print(f"Secret '{args.name}' exists in namespace '{args.namespace}'. Deleting it due to --force option.")
        delete_secret(args.name, args.namespace)
    
    # Generate a new secret and create it as a Kubernetes secret.
    new_secret = generate_secret()
    print(f"Generated new secret: {new_secret}")
    create_secret(args.name, args.namespace, new_secret)
    print(f"Secret '{args.name}' has been created in namespace '{args.namespace}'.")

if __name__ == '__main__':
    main()
