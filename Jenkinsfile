// Global Constants
//
slackNotifyChannelName = "dev-notifications"


pipeline {

    agent {

        docker { 
            image 'netfoundry/ziti-build-pipeline:10'
            registryUrl "https://index.docker.io/v1/"
            registryCredentialsId 'curt_dockerhub'
        }

    }

    options {
        timeout(time: 60, unit: 'MINUTES')
        buildDiscarder(logRotator(numToKeepStr: '3')) 
    }

    environment {
        BRANCH_NAME             = "${BRANCH_NAME}"
        BUILD_NUMBER            = "${BUILD_NUMBER}"
        GOCACHE                 = "${env.WORKSPACE}/.build"
        HOME                    = "${env.WORKSPACE}"
        JFROG_API_KEY           = credentials('ad-tf-var-jfrog-api-key')
        JFROG_CLI_OFFER_CONFIG  = "false"
    }

    stages {

        stage('Setup') {

            steps {

                // Ensure Jenkins agent can manipulate the working directory
                sh "chmod -R 777 ." 

                // Pull down the src
                checkout scm
                                
                // Make a backup of the mod file just in case, for possible later linting
                sh "mkdir .build"
                sh "cp go.mod .build/go.mod.orig"

                // Dump env vars
                sh "printenv | sort"
            }
        }

        stage('Fetch Dependencies') {

            steps {

                sh "go mod download"
            }
        }

        stage('Tests') {

            steps {

                sh "go test ./..."
            }
        }


        stage('Build') {

            steps {

                // Build/publish the binaries
                sh(
                    label: "make",
                    script: """
                        export BRANCH_NAME
                        export BUILD_NUMBER
                        export JFROG_API_KEY
                        export JFROG_CLI_OFFER_CONFIG
                        export AWS_ACCESS_KEY_ID
                        export AWS_SECRET_ACCESS_KEY
                        make
                    """
                )
            }
        }
    }

    post {
        aborted {
            sendSlack("ABORTED",    "${slackNotifyChannelName}")
        }
        unstable {
            sendSlack("UNSTABLE",   "${slackNotifyChannelName}")
        }
        failure {
            sendSlack("FAILED",     "${slackNotifyChannelName}")
        }
        success {
            sendSlack("SUCCESS",    "${slackNotifyChannelName}")
        }
        always {
            sh "chmod -R 777 ." // ensure Jenkins agent can delete the working directory
            deleteDir()
        }
    }
    
}

// -----------------------------------------------------------------------
//  Send a Slack notification describing results of build.
// -----------------------------------------------------------------------
def sendSlack(status, channel) {
  def STATUS_MESSAGE, color
  if ( status == "SUCCESS") {
    STATUS_MESSAGE = ":white_check_mark: Sucessful"
    color = "good"
  } else if ( status == "UNSTABLE" ) {
    STATUS_MESSAGE = ":exclamation: Unstable"
    color = "#ff9800"
  } else if ( status == "ABORTED" ) {
    STATUS_MESSAGE = ":exclamation: Aborted"
    color = "grey"
  } else if ( status == "SMOKEFREE" ) {
    STATUS_MESSAGE = ":smoke-free: Smoke-Free"
    color = "grey"
  }else {
    STATUS_MESSAGE = " :no_entry: Failed"
    color = "danger"
  }
  def duration = currentBuild.durationString.replace(' and counting', '')
  slackSend channel: "${channel}",
    color: "${color}",
    message: "${RUN_DISPLAY_URL}\n"+
    "*Branch*:    ${env.BRANCH_NAME}\n"+
    "*Status*:    ${STATUS_MESSAGE}\n"+
    "*Duration*:  ${duration}\n"
}
