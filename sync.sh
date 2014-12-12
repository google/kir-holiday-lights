cp ./Sequences/* ../Sequences/

# Fix file path for Eric's animation image
sed -e 's/Z:\\git\\kir-holiday-lights/C:\\lights/g' Sequences/DoYouWantToBuildASnowMan.lms > Sequences/DoYouWantToBuildASnowMan.lms.tmp && mv Sequences/DoYouWantToBuildASnowMan.lms.tmp Sequences/DoYouWantToBuildASnowMan.lms

