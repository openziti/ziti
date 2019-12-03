function yaml2json()
{
    ruby -ryaml -rjson -e 'puts JSON.pretty_generate(YAML.load(ARGF))' $*
}

function printUsage()
{
    echo "Usage: $1 [cert to test] [ca pool to use]"
}

function verifyCertAgainstPool()
{
    if [[ "" == "$1" ]]
    then
        printUsage "verifyCertAgainstPool"
        return 1
    fi
    
    if [[ "" == "$2" ]]
    then
        printUsage "verifyCertAgainstPool"
        return 1
    fi

    echo "    Verifying that this certificate:"
    echo "        - $1"
    echo "    is valid for this ca pool:"
    echo "        - $2"
    echo ""
    openssl verify -partial_chain -CAfile "$2" "$1"
    if [ $? -eq 0 ]; then
        echo ""
        echo "============      SUCCESS!      ============"
    else
        echo ""
        echo "============ FAILED TO VALIDATE ============"
    fi
}

function showIssuerAndSubjectForPEM()
{
    echo "Displaying Issuer and Subject for cert pool:"
    echo "    $1"
    openssl crl2pkcs7 -nocrl -certfile $1 | openssl pkcs7 -print_certs -text -noout | grep -E "(Subject|Issuer)"
}
