service {
    name = "ziti"
    id = "ziti"
    port = 6262
    meta {
        build_number= "${build_number}"
        ziti_version= "${ziti_version}"
    }
}